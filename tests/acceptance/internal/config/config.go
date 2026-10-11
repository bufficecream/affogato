// Package config loads acceptance-test settings from the environment.
//
// Defaults target the host + `kubectl port-forward` setup (tests/kind/port-forward.sh).
// The in-cluster Job overrides them with service DNS names (tests/kind/acceptance-job.yaml).
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runner configuration. Secret fields are never printed by String().
type Config struct {
	NATSURL   string // FP_NATS_URL
	NATSCreds string // FP_NATS_CREDS (path to .creds file; optional, secret)

	MinIOEndpoint       string // FP_MINIO_ENDPOINT host:port the runner uses
	MinIOPublicEndpoint string // FP_MINIO_PUBLIC_ENDPOINT URL presigned links must point at
	MinIOAccessKey      string // FP_MINIO_ACCESS_KEY (secret)
	MinIOSecretKey      string // FP_MINIO_SECRET_KEY (secret)
	MinIOUseTLS         bool   // FP_MINIO_USE_TLS
	MinIORegion         string // FP_MINIO_REGION

	QdrantURL    string // FP_QDRANT_URL (REST, e.g. http://localhost:6333)
	QdrantAPIKey string // FP_QDRANT_API_KEY (secret, optional)

	APIBaseURL string // FP_API_BASE_URL

	KubeNamespace string // FP_KUBE_NAMESPACE
	KubeContext   string // FP_KUBE_CONTEXT (optional; empty = current / in-cluster)
	Kubectl       string // FP_KUBECTL binary

	// Workload references for fault injection, "kind/name" (TODO: align with the Helm chart).
	WorkloadStreamWorker string // FP_WORKLOAD_STREAM_WORKER
	WorkloadAIWorker     string // FP_WORKLOAD_AI_WORKER
	WorkloadEmbedder     string // FP_WORKLOAD_EMBEDDER
	WorkloadQdrant       string // FP_WORKLOAD_QDRANT
	WorkloadAPI          string // FP_WORKLOAD_API

	ProducerBin string // FP_PRODUCER_BIN (Producer CLI binary inside the runner image)
	FpctlBin    string // FP_FPCTL_BIN
	FfprobeBin  string // FP_FFPROBE_BIN (remux verification)
	FixturesDir string // FP_FIXTURES_DIR

	StreamWorkerMetricsPort int // FP_STREAM_WORKER_METRICS_PORT (TODO: align with chart)

	EventTimeout     time.Duration // FP_EVENT_TIMEOUT: hot-path "Then" waits
	ColdPathTimeout  time.Duration // FP_COLD_PATH_TIMEOUT: STT + embedding waits
	FinalizeTimeout  time.Duration // FP_FINALIZE_TIMEOUT: idle-timeout finalization waits
	RetryTimeout     time.Duration // FP_RETRY_TIMEOUT: NAK/backoff/DLQ waits
	RolloutTimeout   time.Duration // FP_ROLLOUT_TIMEOUT: kubectl scale / rollout waits
	HTTPTimeout      time.Duration // FP_HTTP_TIMEOUT: per request
	MaxResponseBytes int64         // FP_MAX_RESPONSE_BYTES: cap on bodies the runner buffers
}

// Load reads the environment. It returns all problems at once.
func Load() (*Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (*Config, error) {
	var errs []error
	str := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	dur := func(key string, def time.Duration) time.Duration {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s=%q: want a positive Go duration", key, v))
			return def
		}
		return d
	}
	integer := func(key string, def int64) int64 {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return def
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s=%q: want a positive integer", key, v))
			return def
		}
		return n
	}
	boolean := func(key string, def bool) bool {
		v := strings.TrimSpace(getenv(key))
		if v == "" {
			return def
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s=%q: want a boolean", key, v))
			return def
		}
		return b
	}

	c := &Config{
		NATSURL:   str("FP_NATS_URL", "nats://127.0.0.1:4222"),
		NATSCreds: str("FP_NATS_CREDS", ""),

		MinIOEndpoint:       str("FP_MINIO_ENDPOINT", "127.0.0.1:9000"),
		MinIOPublicEndpoint: str("FP_MINIO_PUBLIC_ENDPOINT", "http://127.0.0.1:9000"),
		MinIOAccessKey:      str("FP_MINIO_ACCESS_KEY", ""),
		MinIOSecretKey:      str("FP_MINIO_SECRET_KEY", ""),
		MinIOUseTLS:         boolean("FP_MINIO_USE_TLS", false),
		MinIORegion:         str("FP_MINIO_REGION", "us-east-1"),

		QdrantURL:    str("FP_QDRANT_URL", "http://127.0.0.1:6333"),
		QdrantAPIKey: str("FP_QDRANT_API_KEY", ""),

		APIBaseURL: str("FP_API_BASE_URL", "http://127.0.0.1:8080"),

		KubeNamespace: str("FP_KUBE_NAMESPACE", "framepulse"),
		KubeContext:   str("FP_KUBE_CONTEXT", ""),
		Kubectl:       str("FP_KUBECTL", "kubectl"),

		WorkloadStreamWorker: str("FP_WORKLOAD_STREAM_WORKER", "deployment/framepulse-stream-worker"),
		WorkloadAIWorker:     str("FP_WORKLOAD_AI_WORKER", "deployment/framepulse-ai-worker"),
		WorkloadEmbedder:     str("FP_WORKLOAD_EMBEDDER", "deployment/framepulse-embedder"),
		WorkloadQdrant:       str("FP_WORKLOAD_QDRANT", "statefulset/framepulse-qdrant"),
		WorkloadAPI:          str("FP_WORKLOAD_API", "deployment/framepulse-serving-api"),

		ProducerBin: str("FP_PRODUCER_BIN", "fp-producer"),
		FpctlBin:    str("FP_FPCTL_BIN", "fpctl"),
		FfprobeBin:  str("FP_FFPROBE_BIN", "ffprobe"),
		FixturesDir: str("FP_FIXTURES_DIR", "../fixtures/media"),

		StreamWorkerMetricsPort: int(integer("FP_STREAM_WORKER_METRICS_PORT", 9090)),

		EventTimeout:    dur("FP_EVENT_TIMEOUT", 30*time.Second),
		ColdPathTimeout: dur("FP_COLD_PATH_TIMEOUT", 2*time.Minute),
		// kind: FINALIZE_IDLE_TIMEOUT=10s + FINALIZE_SWEEP_INTERVAL=2s (spec 2.2, A15) + margin.
		// Against production defaults (5m/30s) set FP_FINALIZE_TIMEOUT=7m.
		FinalizeTimeout: dur("FP_FINALIZE_TIMEOUT", 30*time.Second),
		// F4.1: AI backoff 2+10+30+60 s + processing (~2 min); F4.2 crash loop:
		// 5 x (ack_wait 60 s + restart) ~ 6 min. One bound for both, with margin.
		RetryTimeout:     dur("FP_RETRY_TIMEOUT", 8*time.Minute),
		RolloutTimeout:   dur("FP_ROLLOUT_TIMEOUT", 3*time.Minute),
		HTTPTimeout:      dur("FP_HTTP_TIMEOUT", 10*time.Second),
		MaxResponseBytes: integer("FP_MAX_RESPONSE_BYTES", 8<<20),
	}

	for key, raw := range map[string]string{
		"FP_NATS_URL":              c.NATSURL,
		"FP_MINIO_PUBLIC_ENDPOINT": c.MinIOPublicEndpoint,
		"FP_QDRANT_URL":            c.QdrantURL,
		"FP_API_BASE_URL":          c.APIBaseURL,
	} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s=%q: want an absolute URL", key, raw))
		}
	}
	if strings.Contains(c.MinIOEndpoint, "://") || strings.Contains(c.MinIOEndpoint, "/") {
		errs = append(errs, fmt.Errorf("FP_MINIO_ENDPOINT=%q: want host:port without scheme or path", c.MinIOEndpoint))
	}
	return c, errors.Join(errs...)
}

// String prints the configuration with secrets redacted, for the run log.
func (c *Config) String() string {
	red := func(s string) string {
		if s == "" {
			return "<unset>"
		}
		return "<redacted>"
	}
	return fmt.Sprintf("nats=%s creds=%s minio=%s public=%s tls=%t access_key=%s secret_key=%s "+
		"qdrant=%s qdrant_api_key=%s api=%s namespace=%s kube_context=%q "+
		"timeouts(event=%s cold=%s finalize=%s retry=%s rollout=%s http=%s)",
		c.NATSURL, red(c.NATSCreds), c.MinIOEndpoint, c.MinIOPublicEndpoint, c.MinIOUseTLS,
		red(c.MinIOAccessKey), red(c.MinIOSecretKey), c.QdrantURL, red(c.QdrantAPIKey),
		c.APIBaseURL, c.KubeNamespace, c.KubeContext,
		c.EventTimeout, c.ColdPathTimeout, c.FinalizeTimeout, c.RetryTimeout, c.RolloutTimeout, c.HTTPTimeout)
}
