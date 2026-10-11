package metrics

import "testing"

const sample = `# HELP af_late_chunks_total Chunks that arrived after finalization.
# TYPE af_late_chunks_total counter
af_late_chunks_total{rendition="src",pod="sw-0"} 2
af_late_chunks_total{rendition="src",pod="sw-1"} 1 1728640000000
af_late_chunks_total_created{rendition="src"} 1.7e9
af_consume_latency_seconds_bucket{consumer="hls-stream-worker",le="0.5"} 297
af_other 5
weird{msg="a \"quoted\", value"} 3
`

func TestSum(t *testing.T) {
	v, found, err := Sum([]byte(sample), "af_late_chunks_total", nil)
	if err != nil || !found || v != 3 {
		t.Fatalf("sum = %v %v %v", v, found, err)
	}
	v, _, _ = Sum([]byte(sample), "af_late_chunks_total", map[string]string{"pod": "sw-1"})
	if v != 1 {
		t.Fatalf("filtered = %v", v)
	}
	v, _, _ = Sum([]byte(sample), "af_other", nil)
	if v != 5 {
		t.Fatalf("unlabelled = %v", v)
	}
	v, found, err = Sum([]byte(sample), "weird", map[string]string{"msg": `a "quoted", value`})
	if err != nil || !found || v != 3 {
		t.Fatalf("escaped label = %v %v %v", v, found, err)
	}
	if _, found, _ := Sum([]byte(sample), "missing_total", nil); found {
		t.Fatal("missing metric reported found")
	}
	if _, _, err := Sum([]byte("broken{a=\"x\" 1\n"), "broken", nil); err == nil {
		t.Fatal("malformed exposition accepted")
	}
}
