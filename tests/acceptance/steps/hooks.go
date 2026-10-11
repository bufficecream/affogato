package steps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/cucumber/godog"

	"github.com/affogato/affogato/tests/acceptance/internal/world"
)

// Tracker counts step outcomes godog does not fail on in non-strict mode. Undefined and
// ambiguous steps always fail the run; pending ones only fail it in strict mode.
type Tracker struct {
	mu        sync.Mutex
	undefined map[string]int
	ambiguous map[string]int
	pending   int
}

func NewTracker() *Tracker {
	return &Tracker{undefined: map[string]int{}, ambiguous: map[string]int{}}
}

func (t *Tracker) record(st *godog.Step, status godog.StepResultStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch status {
	case godog.StepUndefined:
		t.undefined[st.Text]++
	case godog.StepAmbiguous:
		t.ambiguous[st.Text]++
	case godog.StepPending:
		t.pending++
	}
}

// Problems returns undefined/ambiguous step texts, sorted.
func (t *Tracker) Problems() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for s := range t.undefined {
		out = append(out, "undefined: "+s)
	}
	for s := range t.ambiguous {
		out = append(out, "ambiguous: "+s)
	}
	sort.Strings(out)
	return out
}

func (t *Tracker) Pending() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending
}

// InitializeScenario wires hooks and step definitions for one scenario.
func InitializeScenario(sc *godog.ScenarioContext, suite *world.Suite, tr *Tracker, log io.Writer) {
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		return world.With(ctx, world.New(suite, s.Name)), nil
	})
	sc.After(func(ctx context.Context, s *godog.Scenario, scErr error) (context.Context, error) {
		wd := world.From(ctx)
		// Video IDs are what you grep pod logs and MinIO for; print them for real failures.
		failed := scErr != nil && !errors.Is(scErr, godog.ErrPending) && !errors.Is(scErr, godog.ErrSkip)
		if ids := wd.Aliases(); failed && len(ids) > 0 && log != nil {
			fmt.Fprintf(log, "  scenario %q video ids: %v\n", s.Name, ids)
		}
		cctx, cancel := context.WithTimeout(context.Background(), suite.Cfg.RolloutTimeout+time.Minute)
		defer cancel()
		if err := wd.Close(cctx); err != nil {
			return ctx, fmt.Errorf("scenario cleanup: %w", err)
		}
		return ctx, nil
	})
	sc.StepContext().After(func(ctx context.Context, st *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
		if tr != nil {
			tr.record(st, status)
		}
		return ctx, err
	})
	RegisterAll(sc)
}
