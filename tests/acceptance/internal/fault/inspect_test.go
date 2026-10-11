package fault

import (
	"context"
	"strings"
	"testing"
	"time"
)

// scriptedKubectl returns canned output per argument substring.
type scriptedKubectl struct {
	calls [][]string
	out   map[string]string // substring of joined args -> stdout
}

func (s *scriptedKubectl) Run(_ context.Context, args ...string) ([]byte, error) {
	s.calls = append(s.calls, args)
	joined := strings.Join(args, " ")
	for k, v := range s.out {
		if strings.Contains(joined, k) {
			return []byte(v), nil
		}
	}
	return nil, nil
}

const deployJSON = `{"spec":{"selector":{"matchLabels":{"app.kubernetes.io/name":"stream-worker","app.kubernetes.io/instance":"affogato"}},
 "template":{"spec":{"containers":[{"name":"worker","env":[
   {"name":"FINALIZE_IDLE_TIMEOUT","value":"10s"},
   {"name":"FINALIZE_SWEEP_INTERVAL","value":"2s"},
   {"name":"AF_TEST_FAULTS","value":"1"},
   {"name":"MINIO_SECRET","valueFrom":{"secretKeyRef":{"name":"m","key":"k"}}}]}]}}}}`

func TestDeploymentEnv(t *testing.T) {
	k := &scriptedKubectl{out: map[string]string{"get deployment/affogato-stream-worker -o json": deployJSON}}
	inj, _ := NewInjector(k, "affogato", time.Minute)
	w := Workload{"deployment", "affogato-stream-worker"}
	ctx := context.Background()
	if v, err := inj.DeploymentEnv(ctx, w, "FINALIZE_IDLE_TIMEOUT"); err != nil || v != "10s" {
		t.Errorf("FINALIZE_IDLE_TIMEOUT = %q, %v", v, err)
	}
	if v, err := inj.DeploymentEnv(ctx, w, "AF_TEST_FAULTS"); err != nil || v != "1" {
		t.Errorf("AF_TEST_FAULTS = %q, %v", v, err)
	}
	if _, err := inj.DeploymentEnv(ctx, w, "MINIO_SECRET"); err == nil || !strings.Contains(err.Error(), "valueFrom") {
		t.Errorf("valueFrom env: %v", err)
	}
	if _, err := inj.DeploymentEnv(ctx, w, "NOT_SET"); err == nil {
		t.Error("missing env accepted")
	}
	if _, err := inj.DeploymentEnv(ctx, w, "BAD NAME; rm"); err == nil {
		t.Error("invalid env name accepted")
	}
}

func TestPodRestarts(t *testing.T) {
	pods := `{"items":[
	 {"status":{"containerStatuses":[{"restartCount":3,"lastState":{"terminated":{"exitCode":70}}}]}},
	 {"status":{"containerStatuses":[{"restartCount":2,"lastState":{"terminated":{"exitCode":70}}}]}},
	 {"status":{"containerStatuses":[{"restartCount":0,"lastState":{}}]}}]}`
	k := &scriptedKubectl{out: map[string]string{
		"get deployment/affogato-ai-worker -o json": deployJSON,
		"get pods -l": pods,
	}}
	inj, _ := NewInjector(k, "affogato", time.Minute)
	r, err := inj.PodRestarts(context.Background(), Workload{"deployment", "affogato-ai-worker"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 5 || r.LastExitCodes[70] != 2 || r.Pods != 3 {
		t.Errorf("restarts = %+v", r)
	}
	last := strings.Join(k.calls[len(k.calls)-1], " ")
	if !strings.Contains(last, "-l app.kubernetes.io/instance=affogato,app.kubernetes.io/name=stream-worker") {
		t.Errorf("selector not deterministic/sorted: %s", last)
	}
	if _, err := labelSelector(map[string]string{"a": "b,c"}); err == nil {
		t.Error("label value with comma accepted")
	}
	if _, err := labelSelector(nil); err == nil {
		t.Error("empty selector accepted")
	}
}

func TestVerifyHealthyOnRestore(t *testing.T) {
	k := &scriptedKubectl{}
	inj, _ := NewInjector(k, "affogato", time.Minute)
	inj.VerifyHealthyOnRestore(Workload{"deployment", "affogato-ai-worker"})
	if p := inj.Pending(); len(p) != 1 || !strings.Contains(p[0], "healthy") {
		t.Fatalf("pending = %v", p)
	}
	if err := inj.RestoreAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(k.calls[0], " ")
	if got != "--namespace affogato rollout status deployment/affogato-ai-worker --timeout=1m0s" {
		t.Errorf("restore ran %q", got)
	}
}
