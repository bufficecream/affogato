package latency

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestPercentileNearestRank(t *testing.T) {
	// 1..100 ms: nearest rank p95 = 95th value, p99 = 99th, p50 = 50th, p100 = max.
	r := NewRecorder("t")
	for i := 100; i >= 1; i-- { // insert unsorted
		_ = r.Add(ms(i))
	}
	for p, want := range map[float64]time.Duration{50: ms(50), 95: ms(95), 99: ms(99), 100: ms(100), 0.5: ms(1), 99.9: ms(100)} {
		got, err := r.Percentile(p)
		if err != nil || got != want {
			t.Errorf("p%v = %v, %v; want %v", p, got, err, want)
		}
	}
}

func TestPercentileSmallSamples(t *testing.T) {
	// 50 samples (cold-path @load): p95 = rank ceil(47.5) = 48th value.
	s := make([]time.Duration, 50)
	for i := range s {
		s[i] = ms(i + 1)
	}
	if got, _ := Percentile(s, 95); got != ms(48) {
		t.Errorf("p95 of 50 = %v, want 48ms", got)
	}
	// 300 samples (hot-path @load): p99 = rank 297.
	s = make([]time.Duration, 300)
	for i := range s {
		s[i] = ms(i + 1)
	}
	if got, _ := Percentile(s, 99); got != ms(297) {
		t.Errorf("p99 of 300 = %v, want 297ms", got)
	}
	if got, _ := Percentile([]time.Duration{ms(7)}, 99); got != ms(7) {
		t.Errorf("single sample p99 = %v", got)
	}
}

func TestPercentileErrors(t *testing.T) {
	if _, err := Percentile(nil, 95); !errors.Is(err, ErrNoSamples) {
		t.Errorf("empty: %v", err)
	}
	for _, p := range []float64{0, -1, 100.1} {
		if _, err := Percentile([]time.Duration{1}, p); err == nil {
			t.Errorf("p=%v accepted", p)
		}
	}
	if err := NewRecorder("x").Add(-time.Millisecond); err == nil {
		t.Error("negative sample accepted")
	}
}

func TestAssertAtMost(t *testing.T) {
	r := NewRecorder("hot")
	for i := 1; i <= 100; i++ {
		_ = r.Add(ms(i * 10)) // 10..1000 ms
	}
	if err := r.AssertAtMost(95, 1500*time.Millisecond, 50); err != nil {
		t.Errorf("within budget: %v", err)
	}
	if err := r.AssertAtMost(99, 500*time.Millisecond, 50); err == nil {
		t.Error("p99 990ms passed a 500ms budget")
	}
	if err := r.AssertAtMost(99, time.Hour, 1000); err == nil {
		t.Error("too few samples accepted")
	}
}

func TestSummaryAndConcurrency(t *testing.T) {
	r := NewRecorder("c")
	var wg sync.WaitGroup
	for i := 1; i <= 200; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _ = r.Add(ms(i)) }(i)
	}
	wg.Wait()
	s, err := r.Summary()
	if err != nil || s.Count != 200 || s.Min != ms(1) || s.Max != ms(200) || s.P99 != ms(198) {
		t.Fatalf("summary = %+v, %v", s, err)
	}
	if s.Mean != 100500*time.Microsecond {
		t.Errorf("mean = %v", s.Mean)
	}
	start := time.Now()
	_ = r.Between(start, start.Add(time.Second))
	if r.Count() != 201 {
		t.Error("Between not recorded")
	}
}

// Regression: ceil(p/100*N) in float64 overshoots by one rank for some (p, N), e.g.
// 0.07*100 = 7.000000000000001 and 0.999*1000 = 999.0000000000001. The rank must equal
// the exact integer ceil(p*N/100) for every N.
func TestPercentileRankHasNoFloatOvershoot(t *testing.T) {
	for _, tc := range []struct {
		pTenths int64 // percentile * 10, so 99.9 is exact
	}{{70}, {290}, {500}, {570}, {900}, {950}, {990}, {999}} {
		p := float64(tc.pTenths) / 10
		for n := 1; n <= 3000; n++ {
			s := make([]time.Duration, n)
			for i := range s {
				s[i] = time.Duration(i+1) * time.Millisecond // value == rank
			}
			got, err := Percentile(s, p)
			if err != nil {
				t.Fatal(err)
			}
			wantRank := (tc.pTenths*int64(n) + 999) / 1000
			if got != time.Duration(wantRank)*time.Millisecond {
				t.Fatalf("p%v of %d samples: rank %d, want %d", p, n, got/time.Millisecond, wantRank)
			}
		}
	}
}
