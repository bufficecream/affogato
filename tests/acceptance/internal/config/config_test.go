package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := load(env(nil))
	if err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	if c.NATSURL != "nats://127.0.0.1:4222" || c.KubeNamespace != "framepulse" || c.EventTimeout != 30*time.Second {
		t.Errorf("unexpected defaults: %+v", c)
	}
	// kind values (spec 2.2 / A15): FINALIZE_IDLE_TIMEOUT=10s, FINALIZE_SWEEP_INTERVAL=2s.
	// The wait must cover idle + one sweep with margin, yet stay far below the 5 m
	// production default so a cluster deployed with production values fails fast.
	if c.FinalizeTimeout <= 10*time.Second+2*time.Second || c.FinalizeTimeout > time.Minute {
		t.Errorf("FinalizeTimeout %s: want > 12s (idle 10s + sweep 2s) and <= 1m", c.FinalizeTimeout)
	}
	// F4.2 crash loop: 5 crashed deliveries x (ai-indexer ack_wait 60 s + restart).
	if c.RetryTimeout < 6*time.Minute {
		t.Errorf("RetryTimeout %s shorter than the ~6 min AI crash loop (F4.2)", c.RetryTimeout)
	}
}

func TestOverrides(t *testing.T) {
	c, err := load(env(map[string]string{
		"FP_NATS_URL":           "nats://nats.framepulse.svc:4222",
		"FP_EVENT_TIMEOUT":      "45s",
		"FP_MINIO_USE_TLS":      "true",
		"FP_MAX_RESPONSE_BYTES": "1024",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.NATSURL != "nats://nats.framepulse.svc:4222" || c.EventTimeout != 45*time.Second || !c.MinIOUseTLS || c.MaxResponseBytes != 1024 {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestInvalidValuesAreAllReported(t *testing.T) {
	_, err := load(env(map[string]string{
		"FP_EVENT_TIMEOUT":         "soon",
		"FP_RETRY_TIMEOUT":         "-1s",
		"FP_MINIO_USE_TLS":         "maybe",
		"FP_API_BASE_URL":          "localhost:8080",
		"FP_MINIO_ENDPOINT":        "http://minio:9000",
		"FP_MAX_RESPONSE_BYTES":    "0",
		"FP_MINIO_PUBLIC_ENDPOINT": "::bad",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, key := range []string{"FP_EVENT_TIMEOUT", "FP_RETRY_TIMEOUT", "FP_MINIO_USE_TLS",
		"FP_API_BASE_URL", "FP_MINIO_ENDPOINT", "FP_MAX_RESPONSE_BYTES", "FP_MINIO_PUBLIC_ENDPOINT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestStringRedactsSecrets(t *testing.T) {
	c, err := load(env(map[string]string{
		"FP_MINIO_ACCESS_KEY": "AKIA-TEST-ACCESS",
		"FP_MINIO_SECRET_KEY": "super-secret-value",
		"FP_QDRANT_API_KEY":   "qdrant-secret",
		"FP_NATS_CREDS":       "/var/run/secrets/nats.creds",
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := c.String()
	for _, secret := range []string{"AKIA-TEST-ACCESS", "super-secret-value", "qdrant-secret", "nats.creds"} {
		if strings.Contains(s, secret) {
			t.Errorf("String() leaks %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "<redacted>") {
		t.Errorf("String() has no redaction marker: %s", s)
	}
}
