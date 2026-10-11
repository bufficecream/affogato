package steps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	gherkin "github.com/cucumber/gherkin/go/v42"
	"github.com/cucumber/godog"
	messages "github.com/cucumber/messages/go/v34"

	"github.com/affogato/affogato/tests/acceptance/internal/config"
	"github.com/affogato/affogato/tests/acceptance/internal/payload"
	"github.com/affogato/affogato/tests/acceptance/internal/world"
)

const featuresDir = "../features"

type recorded struct {
	expr *regexp.Regexp
	fn   reflect.Value
	used int
}

type recorder struct{ defs []*recorded }

func (r *recorder) Step(expr, fn interface{}) {
	var re *regexp.Regexp
	switch e := expr.(type) {
	case string:
		re = regexp.MustCompile(e)
	case *regexp.Regexp:
		re = e
	default:
		panic(fmt.Sprintf("unsupported expression type %T", expr))
	}
	r.defs = append(r.defs, &recorded{expr: re, fn: reflect.ValueOf(fn)})
}

type pickleStep struct {
	feature, scenario string
	step              *messages.PickleStep
}

func loadPickles(t *testing.T) []pickleStep {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(featuresDir, "*.feature"))
	if err != nil || len(files) != 7 {
		t.Fatalf("want 7 feature files, got %d (%v)", len(files), err)
	}
	n := 0
	newID := func() string { n++; return strconv.Itoa(n) }
	var out []pickleStep
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := gherkin.ParseGherkinDocument(bytes.NewReader(b), newID)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, p := range gherkin.Pickles(*doc, f, newID) {
			for _, s := range p.Steps {
				out = append(out, pickleStep{feature: filepath.Base(f), scenario: p.Name, step: s})
			}
		}
	}
	return out
}

func testWorldContext(t *testing.T) context.Context {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return world.With(context.Background(), world.New(&world.Suite{Cfg: cfg, Codec: payload.Default}, "coverage"))
}

// TestEveryStepDefinedOncePendingAndInvocable is the infrastructure-free equivalent of a
// godog dry run, but stricter: godog skips steps after the first pending one, so argument
// conversion errors and wrong spec text in later steps would go unnoticed there.
func TestEveryStepDefinedOncePendingAndInvocable(t *testing.T) {
	rec := &recorder{}
	RegisterAll(rec)
	steps := loadPickles(t)
	if len(steps) == 0 {
		t.Fatal("no steps parsed")
	}
	for _, ps := range steps {
		var matches []*recorded
		for _, d := range rec.defs {
			if d.expr.MatchString(ps.step.Text) {
				matches = append(matches, d)
			}
		}
		where := fmt.Sprintf("%s / %q / %q", ps.feature, ps.scenario, ps.step.Text)
		switch len(matches) {
		case 0:
			t.Errorf("undefined step: %s", where)
			continue
		case 1:
		default:
			var exprs []string
			for _, m := range matches {
				exprs = append(exprs, m.expr.String())
			}
			t.Errorf("ambiguous step: %s matches %v", where, exprs)
			continue
		}
		d := matches[0]
		d.used++
		err := invoke(testWorldContext(t), d, ps.step)
		if !errors.Is(err, godog.ErrPending) {
			t.Errorf("step %s: want ErrPending, got %v", where, err)
		}
	}
	for _, d := range rec.defs {
		if d.used == 0 {
			t.Errorf("step definition never used by any feature: %s", d.expr)
		}
	}
	t.Logf("%d pickle steps checked against %d step definitions", len(steps), len(rec.defs))
}

// invoke calls a step function the way godog does: ctx first, then regex groups converted
// to the parameter types, then the DataTable/DocString argument if present.
func invoke(ctx context.Context, d *recorded, st *messages.PickleStep) error {
	groups := d.expr.FindStringSubmatch(st.Text)[1:]
	ft := d.fn.Type()
	if ft.Kind() != reflect.Func || ft.NumIn() == 0 || ft.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		return fmt.Errorf("step func must take context.Context first: %s", ft)
	}
	want := ft.NumIn() - 1
	var extra any
	if st.Argument != nil {
		switch {
		case st.Argument.DataTable != nil:
			extra = st.Argument.DataTable
		case st.Argument.DocString != nil:
			extra = st.Argument.DocString
		}
	}
	got := len(groups)
	if extra != nil {
		got++
	}
	if got != want {
		return fmt.Errorf("%s: func takes %d args, step provides %d", d.expr, want, got)
	}
	args := []reflect.Value{reflect.ValueOf(ctx)}
	for i, g := range groups {
		pt := ft.In(i + 1)
		switch pt.Kind() {
		case reflect.String:
			args = append(args, reflect.ValueOf(g).Convert(pt))
		case reflect.Int, reflect.Int64:
			n, err := strconv.ParseInt(g, 10, 64)
			if err != nil {
				return fmt.Errorf("arg %d %q: %w", i, g, err)
			}
			args = append(args, reflect.ValueOf(n).Convert(pt))
		case reflect.Float64:
			f, err := strconv.ParseFloat(g, 64)
			if err != nil {
				return fmt.Errorf("arg %d %q: %w", i, g, err)
			}
			args = append(args, reflect.ValueOf(f))
		default:
			return fmt.Errorf("arg %d: unsupported parameter type %s", i, pt)
		}
	}
	if extra != nil {
		args = append(args, reflect.ValueOf(extra))
	}
	out := d.fn.Call(args)
	errV := out[len(out)-1]
	if errV.IsNil() {
		return nil
	}
	return errV.Interface().(error)
}

// TestFeaturesMatchSpec enforces that feature files are verbatim copies of the spec's
// Gherkin blocks, with only the leading comment and tag lines added.
func TestFeaturesMatchSpec(t *testing.T) {
	spec, err := os.ReadFile("../../../affogato_spec.md")
	if err != nil {
		t.Skipf("spec not available (e.g. inside the runner image): %v", err)
	}
	blocks := regexp.MustCompile("(?s)```gherkin\n(.*?)```").FindAllSubmatch(spec, -1)
	files, _ := filepath.Glob(filepath.Join(featuresDir, "*.feature"))
	if len(blocks) != len(files) {
		t.Fatalf("spec has %d gherkin blocks, %d feature files", len(blocks), len(files))
	}
	wantTags := []string{"@hot-path", "@finalization", "@cold-path", "@failures", "@idempotency", "@search", "@retention"}
	for i, f := range files { // Glob sorts; files are numbered in spec order
		b, _ := os.ReadFile(f)
		lines := strings.SplitAfter(string(b), "\n")
		k := 0
		for k < len(lines) && (strings.HasPrefix(lines[k], "#") || strings.HasPrefix(lines[k], "@")) {
			if strings.HasPrefix(lines[k], "@") && strings.TrimSpace(lines[k]) != wantTags[i] {
				t.Errorf("%s: tag line %q, want %q", f, strings.TrimSpace(lines[k]), wantTags[i])
			}
			k++
		}
		body := strings.Join(lines[k:], "")
		if body != string(blocks[i][1]) {
			t.Errorf("%s differs from spec section 4 block %d", f, i+1)
		}
	}
}

func TestArgHelpers(t *testing.T) {
	if got := quotedList(`"000000.ts", "000001.ts" and "000003.ts"`); !reflect.DeepEqual(got, []string{"000000.ts", "000001.ts", "000003.ts"}) {
		t.Errorf("quotedList = %v", got)
	}
	if got, err := intList("2, 0 and 1"); err != nil || !reflect.DeepEqual(got, []int{2, 0, 1}) {
		t.Errorf("intList = %v %v", got, err)
	}
	if _, err := intList("1, x"); err == nil {
		t.Error("intList accepted garbage")
	}
	if n, err := segmentURIArg("000004.ts"); err != nil || n != 4 {
		t.Errorf("segmentURIArg = %d %v", n, err)
	}
	for _, bad := range []string{"4.ts", "000004.m3u8", "../000004.ts"} {
		if _, err := segmentURIArg(bad); err == nil {
			t.Errorf("segmentURIArg(%q) accepted", bad)
		}
	}
	ctx := testWorldContext(t)
	if _, err := subjectArg(ctx, "video.chunks.>"); err == nil {
		t.Error("wildcard subject accepted")
	}
	if _, err := kvKeyArg(ctx, "V.hd"); err == nil {
		t.Error("unknown rendition accepted")
	}
	if _, err := apiPathArg(ctx, "http://evil/api/v1"); err == nil {
		t.Error("absolute URL accepted as API path")
	}
	if _, err := objectArg(ctx, "af-media/videos/V/../../x"); err == nil {
		t.Error("traversal accepted")
	}
}
