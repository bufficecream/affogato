// Package latency records samples for @load scenarios and computes percentiles.
//
// Percentiles use the nearest-rank method: the p-th percentile of N sorted samples is the
// sample at rank ceil(p/100 * N). It always returns an observed value, never interpolates,
// and is conservative for small N (e.g. p99 of 50 samples is the maximum).
package latency

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

var ErrNoSamples = errors.New("latency: no samples")

type Recorder struct {
	name    string
	mu      sync.Mutex
	samples []time.Duration
}

func NewRecorder(name string) *Recorder { return &Recorder{name: name} }

// Add records one sample; negative samples (clock skew) are rejected.
func (r *Recorder) Add(d time.Duration) error {
	if d < 0 {
		return fmt.Errorf("latency %s: negative sample %s (clock skew?)", r.name, d)
	}
	r.mu.Lock()
	r.samples = append(r.samples, d)
	r.mu.Unlock()
	return nil
}

// Between records end - start.
func (r *Recorder) Between(start, end time.Time) error { return r.Add(end.Sub(start)) }

func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.samples)
}

func (r *Recorder) sorted() []time.Duration {
	r.mu.Lock()
	s := append([]time.Duration(nil), r.samples...)
	r.mu.Unlock()
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

// Percentile returns the nearest-rank percentile, p in (0, 100].
func (r *Recorder) Percentile(p float64) (time.Duration, error) {
	return Percentile(r.sorted(), p)
}

// Percentile computes the nearest-rank percentile of already sorted samples.
func Percentile(sorted []time.Duration, p float64) (time.Duration, error) {
	if len(sorted) == 0 {
		return 0, ErrNoSamples
	}
	if math.IsNaN(p) || p <= 0 || p > 100 {
		return 0, fmt.Errorf("latency: percentile %v out of range (0,100]", p)
	}
	// Multiply before dividing and tolerate float noise: ceil(0.07*100) is 8 in float64
	// (0.07*100 = 7.000000000000001), which would pick the wrong sample.
	rank := int(math.Ceil(p*float64(len(sorted))/100 - 1e-9))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1], nil
}

// Summary is a printable digest of a recorder.
type Summary struct {
	Name           string
	Count          int
	Min, Max, Mean time.Duration
	P50, P95, P99  time.Duration
}

func (r *Recorder) Summary() (Summary, error) {
	s := r.sorted()
	if len(s) == 0 {
		return Summary{Name: r.name}, ErrNoSamples
	}
	var total time.Duration
	for _, d := range s {
		total += d
	}
	p50, _ := Percentile(s, 50)
	p95, _ := Percentile(s, 95)
	p99, _ := Percentile(s, 99)
	return Summary{Name: r.name, Count: len(s), Min: s[0], Max: s[len(s)-1], Mean: total / time.Duration(len(s)), P50: p50, P95: p95, P99: p99}, nil
}

func (s Summary) String() string {
	return fmt.Sprintf("%s: n=%d min=%s p50=%s p95=%s p99=%s max=%s mean=%s",
		s.Name, s.Count, s.Min, s.P50, s.P95, s.P99, s.Max, s.Mean)
}

// AssertAtMost fails when the p-th percentile exceeds budget. minSamples guards against
// asserting a p99 on too few samples to mean anything.
func (r *Recorder) AssertAtMost(p float64, budget time.Duration, minSamples int) error {
	if n := r.Count(); n < minSamples {
		return fmt.Errorf("latency %s: %d samples, need at least %d for p%v", r.name, n, minSamples, p)
	}
	got, err := r.Percentile(p)
	if err != nil {
		return err
	}
	if got > budget {
		sum, _ := r.Summary()
		return fmt.Errorf("latency %s: p%v = %s exceeds %s (%s)", r.name, p, got, budget, sum)
	}
	return nil
}
