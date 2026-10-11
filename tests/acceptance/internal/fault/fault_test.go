package fault

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeKubectl struct {
	calls    [][]string
	replicas string
	failOn   string // substring of the joined args that should fail
}

func (f *fakeKubectl) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	joined := strings.Join(args, " ")
	if f.failOn != "" && strings.Contains(joined, f.failOn) {
		return nil, errors.New("boom")
	}
	if strings.Contains(joined, "jsonpath={.spec.replicas}") {
		return []byte(f.replicas), nil
	}
	return nil, nil
}

func (f *fakeKubectl) verbs() []string {
	var v []string
	for _, c := range f.calls {
		v = append(v, strings.Join(c[2:], " ")) // drop --namespace ns
	}
	return v
}

func TestParseWorkload(t *testing.T) {
	w, err := ParseWorkload("sts/framepulse-qdrant")
	if err != nil || w != (Workload{"statefulset", "framepulse-qdrant"}) {
		t.Fatalf("got %v %v", w, err)
	}
	for _, bad := range []string{
		"framepulse-qdrant",
		"pod/x",
		"deployment/",
		"deployment/--all-namespaces",
		"deployment/x --replicas=9",
		"deployment/UPPER",
		"deployment/a/b",
	} {
		if _, err := ParseWorkload(bad); err == nil {
			t.Errorf("ParseWorkload(%q) accepted", bad)
		}
	}
}

func TestScaleDownAndRestore(t *testing.T) {
	f := &fakeKubectl{replicas: "3"}
	inj, err := NewInjector(f, "framepulse", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	qdrant, _ := ParseWorkload("statefulset/framepulse-qdrant")
	if err := inj.ScaleDown(ctx, qdrant); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.calls {
		if c[0] != "--namespace" || c[1] != "framepulse" {
			t.Fatalf("call without namespace scoping: %v", c)
		}
	}
	if got := inj.Pending(); len(got) != 1 || !strings.Contains(got[0], "back to 3") {
		t.Fatalf("pending = %v", got)
	}
	f.calls = nil
	if err := inj.RestoreAll(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"scale statefulset/framepulse-qdrant --replicas=3",
		"rollout status statefulset/framepulse-qdrant --timeout=1m0s",
	}
	if !reflect.DeepEqual(f.verbs(), want) {
		t.Fatalf("restore calls = %v", f.verbs())
	}
	if len(inj.Pending()) != 0 {
		t.Fatal("undo list not cleared")
	}
	f.calls = nil
	if err := inj.RestoreAll(ctx); err != nil || len(f.calls) != 0 {
		t.Fatal("second RestoreAll re-applied undos")
	}
}

func TestRestoreOrderAndErrors(t *testing.T) {
	f := &fakeKubectl{replicas: "1"}
	inj, _ := NewInjector(f, "framepulse", time.Minute)
	ctx := context.Background()
	ai, _ := ParseWorkload("deployment/framepulse-ai-worker")
	if err := inj.PauseKEDA(ctx, "framepulse-ai-worker", 0); err != nil {
		t.Fatal(err)
	}
	if err := inj.ScaleDown(ctx, ai); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	f.failOn = "rollout status"
	err := inj.RestoreAll(ctx)
	if err == nil || !strings.Contains(err.Error(), "framepulse-ai-worker") {
		t.Fatalf("want restore error, got %v", err)
	}
	// LIFO: scale back first, then unpause KEDA; the unpause still ran despite the error.
	v := f.verbs()
	if len(v) != 3 || !strings.HasPrefix(v[0], "scale deployment/framepulse-ai-worker --replicas=1") ||
		v[2] != "annotate scaledobject.keda.sh/framepulse-ai-worker autoscaling.keda.sh/paused-replicas-" {
		t.Fatalf("restore order = %v", v)
	}
}

func TestInjectorValidation(t *testing.T) {
	if _, err := NewInjector(&fakeKubectl{}, "--all-namespaces", time.Minute); err == nil {
		t.Error("flag-like namespace accepted")
	}
	inj, _ := NewInjector(&fakeKubectl{}, "framepulse", time.Minute)
	if err := inj.PauseKEDA(context.Background(), "x --overwrite", 0); err == nil {
		t.Error("invalid scaledobject accepted")
	}
	f := &fakeKubectl{replicas: "abc"}
	inj, _ = NewInjector(f, "framepulse", time.Minute)
	w, _ := ParseWorkload("deployment/x")
	if err := inj.ScaleDown(context.Background(), w); err == nil {
		t.Error("garbage replicas accepted")
	}
	if len(inj.Pending()) != 0 {
		t.Error("undo registered although scale never happened")
	}
}

// Terminated pods (Succeeded init Jobs, Failed earlier runner Jobs) never become Ready, so
// the readiness wait must exclude both phases or the Background step hangs until timeout.
func TestWaitReadyExcludesTerminatedPods(t *testing.T) {
	f := &fakeKubectl{}
	inj, err := NewInjector(f, "framepulse", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := inj.WaitReady(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"wait deployment --all --for=condition=Available --timeout=2m0s",
		"wait pod --all --field-selector=status.phase!=Succeeded,status.phase!=Failed --for=condition=Ready --timeout=2m0s",
	}
	if got := f.verbs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("kubectl calls:\n got %q\nwant %q", got, want)
	}
	for _, c := range f.calls {
		if c[0] != "--namespace" || c[1] != "framepulse" {
			t.Errorf("call not namespaced: %q", c)
		}
	}
}
