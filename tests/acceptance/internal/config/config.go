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
	NATSURL   string // AF_NATS_URL
	NATSCreds string // AF_NATS_CREDS (path to .creds file; optional, secret)

	MinIOEndpoint       string // AF_MINIO_ENDPOINT host:port the runner uses
	MinIOPublicEndpoint string // AF_MINIO_PUBLIC_ENDPOINT URL presigned links must point at
	MinIOAccessKey      string // AF_MINIO_ACCESS_KEY (secret)
	MinIOSecretKey      string // AF_MINIO_SECRET_KEY (secret)
	MinIOUseTLS         bool   // AF_MINIO_USE_TLS
	MinIORegion         string // AF_MINIO_REGION

	QdrantURL    string // AF_QDRANT_URL (REST, e.g. http://localhost:6333)
	QdrantAPIKey string // AF_QDRANT_API_KEY (secret, optional)

	APIBaseURL string // AF_API_BASE_URL

	KubeNamespace string // AF_KUBE_NAMESPACE
	KubeContext   string // AF_KUBE_CONTEXT (optional; empty = current / in-cluster)
	Kubectl       string // AF_KUBECTL binary

	// Workload references for fault injection, "kind/name" (TODO: align with the Helm chart).
	WorkloadStreamWorker string // AF_WORKLOAD_STREAM_WORKER
	WorkloadAIWorker     string // AF_WORKLOAD_AI_WORKER
	WorkloadEmbedder     string // AF_WORKLOAD_EMBEDDER
	WorkloadQdrant       string // AF_WORKLOAD_QDRANT
	WorkloadAPI          string // AF_WORKLOAD_API

	ProducerBin string // AF_PRODUCER_BIN (Producer CLI binary inside the runner image)
	AfctlBin    string // AF_AFCTL_BIN
	FfprobeBin  string // AF_FFPROBE_BIN (remux verification)
	FixturesDir string // AF_FIXTURES_DIR

	StreamWorkerMetricsPort int // AF_STREAM_WORKER_METRICS_PORT (TODO: align with chart)

	EventTimeout     time.Duration // AF_EVENT_TIMEOUT: hot-path "Then" waits
	ColdPathTimeout  time.Duration // AF_COLD_PATH_TIMEOUT: STT + embedding waits
	FinalizeTimeout  time.Duration // AF_FINALIZE_TIMEOUT: idle-timeout finalization waits
	RetryTimeout     time.Duration // AF_RETRY_TIMEOUT: NAK/backoff/DLQ waits
	RolloutTimeout   time.Duration // AF_ROLLOUT_TIMEOUT: kubectl scale / rollout waits
	HTTPTimeout      time.Duration // AF_HTTP_TIMEOUT: per request
	MaxResponseBytes int64         // AF_MAX_RESPONSE_BYTES: cap on bodies the runner buffers
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
		NATSURL:   str("AF_NATS_URL", "nats://127.0.0.1:4222"),
		NATSCreds: str("AF_NATS_CREDS", ""),

		MinIOEndpoint:       str("AF_MINIO_ENDPOINT", "127.0.0.1:9000"),
		MinIOPublicEndpoint: str("AF_MINIO_PUBLIC_ENDPOINT", "http://127.0.0.1:9000"),
		MinIOAccessKey:      str("AF_MINIO_ACCESS_KEY", ""),
		MinIOSecretKey:      str("AF_MINIO_SECRET_KEY", ""),
		MinIOUseTLS:         boolean("AF_MINIO_USE_TLS", false),
		MinIORegion:         str("AF_MINIO_REGION", "us-east-1"),

		QdrantURL:    str("AF_QDRANT_URL", "http://127.0.0.1:6333"),
		QdrantAPIKey: str("AF_QDRANT_API_KEY", ""),

		APIBaseURL: str("AF_API_BASE_URL", "http://127.0.0.1:8080"),

		KubeNamespace: str("AF_KUBE_NAMESPACE", "affogato"),
		KubeContext:   str("AF_KUBE_CONTEXT", ""),
		Kubectl:       str("AF_KUBECTL", "kubectl"),

		WorkloadStreamWorker: str("AF_WORKLOAD_STREAM_WORKER", "deployment/affogato-stream-worker"),
		WorkloadAIWorker:     str("AF_WORKLOAD_AI_WORKER", "deployment/affogato-ai-worker"),
		WorkloadEmbedder:     str("AF_WORKLOAD_EMBEDDER", "deployment/affogato-embedder"),
		WorkloadQdrant:       str("AF_WORKLOAD_QDRANT", "statefulset/affogato-qdrant"),
		WorkloadAPI:          str("AF_WORKLOAD_API", "deployment/affogato-serving-api"),

		ProducerBin: str("AF_PRODUCER_BIN", "af-producer"),
		AfctlBin:    str("AF_AFCTL_BIN", "afctl"),
		FfprobeBin:  str("AF_FFPROBE_BIN", "ffprobe"),
		FixturesDir: str("AF_FIXTURES_DIR", "../fixtures/media"),

		StreamWorkerMetricsPort: int(integer("AF_STREAM_WORKER_METRICS_PORT", 9090)),

		EventTimeout:    dur("AF_EVENT_TIMEOUT", 30*time.Second),
		ColdPathTimeout: dur("AF_COLD_PATH_TIMEOUT", 2*time.Minute),
		// kind: FINALIZE_IDLE_TIMEOUT=10s + FINALIZE_SWEEP_INTERVAL=2s (spec 2.2, A15) + margin.
		// Against production defaults (5m/30s) set AF_FINALIZE_TIMEOUT=7m.
		FinalizeTimeout: dur("AF_FINALIZE_TIMEOUT", 30*time.Second),
		// F4.1: AI backoff 2+10+30+60 s + processing (~2 min); F4.2 crash loop:
		// 5 x (ack_wait 60 s + restart) ~ 6 min. One bound for both, with margin.
		RetryTimeout:     dur("AF_RETRY_TIMEOUT", 8*time.Minute),
		RolloutTimeout:   dur("AF_ROLLOUT_TIMEOUT", 3*time.Minute),
		HTTPTimeout:      dur("AF_HTTP_TIMEOUT", 10*time.Second),
		MaxResponseBytes: integer("AF_MAX_RESPONSE_BYTES", 8<<20),
	}

	for key, raw := range map[string]string{
		"AF_NATS_URL":              c.NATSURL,
		"AF_MINIO_PUBLIC_ENDPOINT": c.MinIOPublicEndpoint,
		"AF_QDRANT_URL":            c.QdrantURL,
		"AF_API_BASE_URL":          c.APIBaseURL,
	} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s=%q: want an absolute URL", key, raw))
		}
	}
	if strings.Contains(c.MinIOEndpoint, "://") || strings.Contains(c.MinIOEndpoint, "/") {
		errs = append(errs, fmt.Errorf("AF_MINIO_ENDPOINT=%q: want host:port without scheme or path", c.MinIOEndpoint))
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
