package fault

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Read-only inspection of workloads, used by steps that check cluster configuration
// (Feature 2 "the Stream Worker runs with FINALIZE_IDLE_TIMEOUT …") or count crashes
// caused by the test-only fault hooks (Feature 4 crash loop, Feature 5 crash before ack).

// workloadSpec is the subset of a Deployment/StatefulSet the helpers read.
type workloadSpec struct {
	Spec struct {
		Selector struct {
			MatchLabels map[string]string `json:"matchLabels"`
		} `json:"selector"`
		Template struct {
			Spec struct {
				Containers []struct {
					Name string `json:"name"`
					Env  []struct {
						Name      string          `json:"name"`
						Value     *string         `json:"value"`
						ValueFrom json.RawMessage `json:"valueFrom"`
					} `json:"env"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

func (i *Injector) workloadSpec(ctx context.Context, w Workload) (workloadSpec, error) {
	var ws workloadSpec
	out, err := i.kubectl(ctx, "get", w.String(), "-o", "json")
	if err != nil {
		return ws, err
	}
	if err := json.Unmarshal(out, &ws); err != nil {
		return ws, fmt.Errorf("%s: decode: %w", w, err)
	}
	return ws, nil
}

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// DeploymentEnv returns the literal value of env var name in the workload's pod template.
// It fails if the variable is missing, set via valueFrom (not checkable as a literal), or
// set to different values in different containers.
func (i *Injector) DeploymentEnv(ctx context.Context, w Workload, name string) (string, error) {
	if !envNameRe.MatchString(name) {
		return "", fmt.Errorf("env name %q invalid", name)
	}
	ws, err := i.workloadSpec(ctx, w)
	if err != nil {
		return "", err
	}
	var found []string
	for _, c := range ws.Spec.Template.Spec.Containers {
		for _, e := range c.Env {
			if e.Name != name {
				continue
			}
			if e.Value == nil {
				return "", fmt.Errorf("%s container %s: %s is set via valueFrom, not a literal", w, c.Name, name)
			}
			found = append(found, *e.Value)
		}
	}
	switch {
	case len(found) == 0:
		return "", fmt.Errorf("%s: env %s not set in the pod template (default applies)", w, name)
	case len(found) > 1:
		for _, v := range found[1:] {
			if v != found[0] {
				return "", fmt.Errorf("%s: env %s differs between containers: %v", w, name, found)
			}
		}
	}
	return found[0], nil
}

// Restarts summarises container restarts of a workload's pods.
type Restarts struct {
	Total int
	// LastExitCodes counts lastState.terminated.exitCode over all containers; the fault
	// hooks exit with code 70, so a hook-induced crash shows up as LastExitCodes[70].
	LastExitCodes map[int]int
	Pods          int
}

type podList struct {
	Items []struct {
		Status struct {
			ContainerStatuses []struct {
				RestartCount int `json:"restartCount"`
				LastState    struct {
					Terminated *struct {
						ExitCode int `json:"exitCode"`
					} `json:"terminated"`
				} `json:"lastState"`
			} `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

// labelSelector renders matchLabels as a deterministic kubectl -l selector.
func labelSelector(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "", fmt.Errorf("workload has no matchLabels")
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.ContainsAny(k+m[k], ",=! ") {
			return "", fmt.Errorf("unsupported label %q=%q", k, m[k])
		}
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ","), nil
}

// PodRestarts sums restartCount over the workload's current pods. Snapshot it before a
// crash-inducing publish and compare afterwards (counts survive container restarts, not
// pod replacement).
func (i *Injector) PodRestarts(ctx context.Context, w Workload) (Restarts, error) {
	ws, err := i.workloadSpec(ctx, w)
	if err != nil {
		return Restarts{}, err
	}
	sel, err := labelSelector(ws.Spec.Selector.MatchLabels)
	if err != nil {
		return Restarts{}, fmt.Errorf("%s: %w", w, err)
	}
	out, err := i.kubectl(ctx, "get", "pods", "-l", sel, "-o", "json")
	if err != nil {
		return Restarts{}, err
	}
	var pl podList
	if err := json.Unmarshal(out, &pl); err != nil {
		return Restarts{}, fmt.Errorf("pods of %s: decode: %w", w, err)
	}
	r := Restarts{LastExitCodes: map[int]int{}, Pods: len(pl.Items)}
	for _, p := range pl.Items {
		for _, cs := range p.Status.ContainerStatuses {
			r.Total += cs.RestartCount
			if t := cs.LastState.Terminated; t != nil {
				r.LastExitCodes[t.ExitCode]++
			}
		}
	}
	return r, nil
}

// VerifyHealthyOnRestore registers an After-hook action that waits for w's rollout to be
// complete (all replicas ready). Scenarios that crash a worker on purpose (fault hooks)
// call it first, so the next scenario never starts against a crash-looping worker.
func (i *Injector) VerifyHealthyOnRestore(w Workload) {
	i.push("verify "+w.String()+" healthy", func(ctx context.Context) error {
		_, err := i.kubectl(ctx, "rollout", "status", w.String(), "--timeout="+i.rolloutTimeout.String())
		return err
	})
}
