// Acceptance test runner for Affogato (spec section 4), executed with godog.
//
// Run from the host (port-forwarded cluster) or in-cluster as a compiled test binary:
//
//	go test -run '^TestFeatures$' . -args -godog.tags='~@load'
//	./acceptance.test -test.run '^TestFeatures$' -godog.format=pretty,junit:/reports/junit.xml
//
// Options come from flags (-godog.*) with environment fallbacks:
//
//	GODOG_TAGS     tag expression           default "~@load" (load runs use "@load")
//	GODOG_FORMAT   formatter(s)             default "pretty"
//	GODOG_PATHS    comma-separated paths    default "features"
//	GODOG_STRICT   fail on pending steps    default false (set true once services exist)
//	GODOG_RANDOM   random seed (0 = off)    default 0
//
// Undefined or ambiguous steps always fail the run, strict or not.
// Connection settings: see internal/config (AF_* variables).
package acceptance

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"

	"github.com/affogato/affogato/tests/acceptance/internal/config"
	"github.com/affogato/affogato/tests/acceptance/internal/fault"
	"github.com/affogato/affogato/tests/acceptance/internal/payload"
	"github.com/affogato/affogato/tests/acceptance/internal/world"
	"github.com/affogato/affogato/tests/acceptance/steps"
)

var opts = godog.Options{
	Output:      colors.Colored(os.Stdout),
	Format:      "pretty",
	Paths:       []string{"features"},
	Tags:        "~@load",
	Concurrency: 1,
}

func init() {
	if err := applyEnv(&opts, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "acceptance:", err)
		os.Exit(2)
	}
	// BindFlags (std flag) rather than BindCommandLineFlags (pflag in godog v0.16), so that
	// `go test` / the compiled test binary parse -godog.* together with -test.* flags.
	godog.BindFlags("godog.", flag.CommandLine, &opts)
}

// applyEnv sets option defaults from GODOG_* variables; flags parsed later override them.
func applyEnv(o *godog.Options, getenv func(string) string) error {
	if v := getenv("GODOG_TAGS"); v != "" {
		o.Tags = v
	}
	if v := getenv("GODOG_FORMAT"); v != "" {
		o.Format = v
	}
	if v := getenv("GODOG_PATHS"); v != "" {
		o.Paths = strings.Split(v, ",")
	}
	if v := getenv("GODOG_STRICT"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("GODOG_STRICT=%q: %w", v, err)
		}
		o.Strict = b
	}
	if v := getenv("GODOG_RANDOM"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("GODOG_RANDOM=%q: %w", v, err)
		}
		o.Randomize = n
	}
	if getenv("NO_COLOR") != "" {
		o.NoColors = true
	}
	return nil
}

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestFeatures(t *testing.T) {
	if opts.Concurrency != 1 {
		// Fault injection scales shared workloads and search results are global; scenarios
		// must not overlap.
		t.Fatalf("godog concurrency %d: acceptance scenarios share cluster state and must run serially", opts.Concurrency)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("configuration: %v", err)
	}
	t.Logf("config: %s", cfg)

	inj, err := fault.NewInjector(fault.ExecRunner{Kubectl: cfg.Kubectl, KubeContext: cfg.KubeContext}, cfg.KubeNamespace, cfg.RolloutTimeout)
	if err != nil {
		t.Fatalf("fault injector: %v", err)
	}
	suite := &world.Suite{Cfg: cfg, Codec: payload.Default, Injector: inj}
	tracker := steps.NewTracker()

	status := godog.TestSuite{
		Name: "affogato-acceptance",
		TestSuiteInitializer: func(tsc *godog.TestSuiteContext) {
			// Safety net: restore anything a scenario's own cleanup failed to restore.
			tsc.AfterSuite(func() {
				ctx, cancel := context.WithTimeout(context.Background(), cfg.RolloutTimeout+time.Minute)
				defer cancel()
				if left := inj.Pending(); len(left) > 0 {
					fmt.Fprintf(os.Stderr, "restoring leftover faults: %v\n", left)
				}
				if err := inj.RestoreAll(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "fault restore failed: %v\n", err)
				}
			})
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			steps.InitializeScenario(sc, suite, tracker, os.Stdout)
		},
		Options: &opts,
	}.Run()

	if problems := tracker.Problems(); len(problems) > 0 {
		t.Errorf("%d undefined/ambiguous steps:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	t.Logf("pending steps: %d (strict=%v)", tracker.Pending(), opts.Strict)
	if status != 0 {
		t.Fatalf("godog suite exited with status %d", status)
	}
}
