// Package fault injects workload outages by scaling Kubernetes workloads with kubectl
// (spec section 4: "Fault-injection steps scale the workload to zero with kubectl and
// restore it afterwards").
//
// Every change registers an undo action; RestoreAll runs them in reverse order and is
// called from the scenario After hook, so a failing scenario never leaves Qdrant or a
// worker scaled down for the next one. Scenarios that inject faults mutate shared
// cluster state and must not run concurrently with other scenarios.
package fault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Runner executes kubectl. The real one shells out; tests use a fake.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ExecRunner runs a kubectl binary. In-cluster it uses the pod's service account.
type ExecRunner struct {
	Kubectl     string
	KubeContext string
}

func (r ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	if r.KubeContext != "" {
		args = append([]string{"--context", r.KubeContext}, args...)
	}
	cmd := exec.CommandContext(ctx, r.Kubectl, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Workload is a scalable resource such as "deployment/affogato-ai-worker".
type Workload struct {
	Kind string // deployment | statefulset
	Name string
}

func (w Workload) String() string { return w.Kind + "/" + w.Name }

var dns1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// ParseWorkload validates "kind/name". Names must be DNS-1123 labels, so values from
// config or step text cannot smuggle extra kubectl flags.
func ParseWorkload(s string) (Workload, error) {
	kind, name, ok := strings.Cut(s, "/")
	if !ok {
		return Workload{}, fmt.Errorf("workload %q: want kind/name", s)
	}
	switch kind {
	case "deployment", "deploy":
		kind = "deployment"
	case "statefulset", "sts":
		kind = "statefulset"
	default:
		return Workload{}, fmt.Errorf("workload %q: kind must be deployment or statefulset", s)
	}
	if !dns1123.MatchString(name) {
		return Workload{}, fmt.Errorf("workload %q: invalid name", s)
	}
	return Workload{Kind: kind, Name: name}, nil
}

// Injector scales workloads in one namespace and remembers how to undo it.
type Injector struct {
	run            Runner
	namespace      string
	rolloutTimeout time.Duration

	mu    sync.Mutex
	undos []undo
}

type undo struct {
	desc string
	fn   func(context.Context) error
}

func NewInjector(r Runner, namespace string, rolloutTimeout time.Duration) (*Injector, error) {
	if !dns1123.MatchString(namespace) {
		return nil, fmt.Errorf("namespace %q invalid", namespace)
	}
	return &Injector{run: r, namespace: namespace, rolloutTimeout: rolloutTimeout}, nil
}

func (i *Injector) kubectl(ctx context.Context, args ...string) ([]byte, error) {
	return i.run.Run(ctx, append([]string{"--namespace", i.namespace}, args...)...)
}

// Replicas returns spec.replicas of a workload.
func (i *Injector) Replicas(ctx context.Context, w Workload) (int, error) {
	out, err := i.kubectl(ctx, "get", w.String(), "-o", "jsonpath={.spec.replicas}")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("%s: unexpected replicas %q", w, out)
	}
	return n, nil
}

// PauseKEDA pins a ScaledObject's replica count with the autoscaling.keda.sh/paused-replicas
// annotation. Without it KEDA would scale a worker back up while the test holds it at 0.
func (i *Injector) PauseKEDA(ctx context.Context, scaledObject string, replicas int) error {
	if !dns1123.MatchString(scaledObject) {
		return fmt.Errorf("scaledobject %q invalid", scaledObject)
	}
	ref := "scaledobject.keda.sh/" + scaledObject
	if _, err := i.kubectl(ctx, "annotate", ref, "autoscaling.keda.sh/paused-replicas="+strconv.Itoa(replicas), "--overwrite"); err != nil {
		return err
	}
	i.push("unpause "+ref, func(ctx context.Context) error {
		_, err := i.kubectl(ctx, "annotate", ref, "autoscaling.keda.sh/paused-replicas-")
		return err
	})
	return nil
}

// ScaleDown scales w to zero and waits until it reports no ready replicas. The undo
// scales it back to its previous replica count and waits for the rollout.
func (i *Injector) ScaleDown(ctx context.Context, w Workload) error {
	prev, err := i.Replicas(ctx, w)
	if err != nil {
		return err
	}
	if _, err := i.kubectl(ctx, "scale", w.String(), "--replicas=0"); err != nil {
		return err
	}
	i.push(fmt.Sprintf("scale %s back to %d", w, prev), func(ctx context.Context) error {
		return i.ScaleTo(ctx, w, prev)
	})
	return i.waitZero(ctx, w)
}

// ScaleTo sets replicas and waits for the rollout to finish (no undo registered).
func (i *Injector) ScaleTo(ctx context.Context, w Workload, replicas int) error {
	if replicas < 0 {
		return fmt.Errorf("negative replicas %d", replicas)
	}
	if _, err := i.kubectl(ctx, "scale", w.String(), "--replicas="+strconv.Itoa(replicas)); err != nil {
		return err
	}
	if replicas == 0 {
		return i.waitZero(ctx, w)
	}
	_, err := i.kubectl(ctx, "rollout", "status", w.String(), "--timeout="+i.rolloutTimeout.String())
	return err
}

func (i *Injector) waitZero(ctx context.Context, w Workload) error {
	_, err := i.kubectl(ctx, "wait", w.String(), "--for=jsonpath={.status.readyReplicas}=0", "--timeout="+i.rolloutTimeout.String())
	if err != nil {
		// readyReplicas is omitted (not 0) once nothing is ready; accept that form too.
		out, gerr := i.kubectl(ctx, "get", w.String(), "-o", "jsonpath={.status.readyReplicas}")
		if gerr == nil && (strings.TrimSpace(string(out)) == "" || strings.TrimSpace(string(out)) == "0") {
			return nil
		}
		return err
	}
	return nil
}

// WaitReady blocks until every Deployment is Available and every non-terminated pod in the
// namespace is Ready (Background step). Terminated pods never become Ready and are
// excluded: Succeeded Job pods (stream/KV init, NACK) and Failed ones, e.g. a previous
// failed af-acceptance-load run kept for ttlSecondsAfterFinished in the same namespace.
func (i *Injector) WaitReady(ctx context.Context) error {
	timeout := "--timeout=" + i.rolloutTimeout.String()
	if _, err := i.kubectl(ctx, "wait", "deployment", "--all", "--for=condition=Available", timeout); err != nil {
		return err
	}
	_, err := i.kubectl(ctx, "wait", "pod", "--all", "--field-selector=status.phase!=Succeeded,status.phase!=Failed", "--for=condition=Ready", timeout)
	return err
}

func (i *Injector) push(desc string, fn func(context.Context) error) {
	i.mu.Lock()
	i.undos = append(i.undos, undo{desc, fn})
	i.mu.Unlock()
}

// Pending lists outstanding undo actions (for logs).
func (i *Injector) Pending() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]string, len(i.undos))
	for k, u := range i.undos {
		out[k] = u.desc
	}
	return out
}

// RestoreAll runs all undo actions in reverse order. It keeps going after errors and
// returns them joined; the list is cleared either way so a retry does not double-apply.
func (i *Injector) RestoreAll(ctx context.Context) error {
	i.mu.Lock()
	undos := i.undos
	i.undos = nil
	i.mu.Unlock()
	var errs []error
	for k := len(undos) - 1; k >= 0; k-- {
		if err := undos[k].fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", undos[k].desc, err))
		}
	}
	return errors.Join(errs...)
}
