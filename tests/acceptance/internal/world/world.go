// Package world holds per-scenario state: video aliases ("V", "W") mapped to fresh
// UUIDv7s, lazily created infrastructure clients, event waiters and cleanup hooks.
//
// Clients connect on first use, so a scenario whose steps are all pending never touches
// the cluster; this is what lets the dry run work without one.
package world

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/framepulse/framepulse/tests/acceptance/internal/config"
	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
	"github.com/framepulse/framepulse/tests/acceptance/internal/events"
	"github.com/framepulse/framepulse/tests/acceptance/internal/fault"
	"github.com/framepulse/framepulse/tests/acceptance/internal/httpclient"
	"github.com/framepulse/framepulse/tests/acceptance/internal/latency"
	"github.com/framepulse/framepulse/tests/acceptance/internal/payload"
	"github.com/framepulse/framepulse/tests/acceptance/internal/qdrant"
	"github.com/framepulse/framepulse/tests/acceptance/internal/s3assert"
)

// Suite holds state shared by all scenarios of one run.
type Suite struct {
	Cfg      *config.Config
	Codec    payload.Codec
	Injector *fault.Injector // shared so AfterSuite can restore anything left behind
}

// World is the state of one scenario.
type World struct {
	Suite *Suite
	Name  string

	mu       sync.Mutex
	aliases  map[string]string
	cleanups []cleanup

	nc      *nats.Conn
	js      jetstream.JetStream
	waiters map[string]*events.Waiter
	s3      *s3assert.Store
	api     *httpclient.Client
	qd      *qdrant.Client
	lat     map[string]*latency.Recorder

	// Last HTTP response, for "the response body …" / "responds the same way" steps.
	LastResponse *httpclient.Response
	// Scratch values shared between steps of one scenario (e.g. measured durations).
	Values map[string]any
}

type cleanup struct {
	name string
	fn   func(context.Context) error
}

func New(s *Suite, scenarioName string) *World {
	return &World{
		Suite:   s,
		Name:    scenarioName,
		aliases: map[string]string{},
		waiters: map[string]*events.Waiter{},
		lat:     map[string]*latency.Recorder{},
		Values:  map[string]any{},
	}
}

type ctxKey struct{}

func With(ctx context.Context, w *World) context.Context { return context.WithValue(ctx, ctxKey{}, w) }

// From returns the scenario's World; it panics if the Before hook did not install one,
// which is a wiring bug rather than a test failure.
func From(ctx context.Context) *World {
	w, ok := ctx.Value(ctxKey{}).(*World)
	if !ok {
		panic("world: no World in context (missing Before hook?)")
	}
	return w
}

// ---- Video aliases ----

var aliasRe = regexp.MustCompile(`^[A-Z]$`)

// Video returns the UUIDv7 for alias, generating it on first use.
func (w *World) Video(alias string) (string, error) {
	if !aliasRe.MatchString(alias) {
		return "", fmt.Errorf("video alias %q: want a single capital letter", alias)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if id, ok := w.aliases[alias]; ok {
		return id, nil
	}
	id, err := contracts.NewVideoID()
	if err != nil {
		return "", err
	}
	w.aliases[alias] = id
	return id, nil
}

// Aliases returns alias -> video_id, sorted by alias, for logging.
func (w *World) Aliases() [][2]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([][2]string, 0, len(w.aliases))
	for a, id := range w.aliases {
		out = append(out, [2]string{a, id})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// Placeholder used in Feature 2 for a video that must not exist.
const UnknownVideoPlaceholder = "{an unknown UUIDv7}"

// aliasTokenRe finds single-letter aliases used as a path/subject/key token:
// "/V/", ".V", "V.src", "\"V:000042\"", "(V)". A letter that is part of a word
// ("VOD", "EXT-X-…") is never a token because its neighbours are letters or '-'.
var aliasTokenRe = regexp.MustCompile(`(^|[/.:"(])([A-Z])($|[/.:")])`)

// Expand replaces alias tokens in spec text with the scenario's video_ids, and the
// unknown-video placeholder with a fresh UUIDv7 that is never used for anything else.
func (w *World) Expand(s string) (string, error) {
	var firstErr error
	if idx := strings.Index(s, UnknownVideoPlaceholder); idx >= 0 {
		id, err := contracts.NewVideoID()
		if err != nil {
			return "", err
		}
		s = s[:idx] + id + s[idx+len(UnknownVideoPlaceholder):]
	}
	// Two passes: adjacent tokens share a delimiter ("V.V" is not a real case, but
	// "/V/" followed by "V.src" in one string is), and ReplaceAll does not overlap.
	for pass := 0; pass < 2; pass++ {
		s = aliasTokenRe.ReplaceAllStringFunc(s, func(m string) string {
			sub := aliasTokenRe.FindStringSubmatch(m)
			id, err := w.Video(sub[2])
			if err != nil && firstErr == nil {
				firstErr = err
			}
			return sub[1] + id + sub[3]
		})
	}
	return s, firstErr
}

// ---- Cleanup ----

// AddCleanup registers fn to run (LIFO) when the scenario ends, pass or fail.
func (w *World) AddCleanup(name string, fn func(context.Context) error) {
	w.mu.Lock()
	w.cleanups = append(w.cleanups, cleanup{name, fn})
	w.mu.Unlock()
}

// Close runs cleanups in reverse order, stops waiters and closes connections.
func (w *World) Close(ctx context.Context) error {
	w.mu.Lock()
	cs := w.cleanups
	w.cleanups = nil
	w.mu.Unlock()
	var errs []error
	for i := len(cs) - 1; i >= 0; i-- {
		if err := cs[i].fn(ctx); err != nil {
			errs = append(errs, fmt.Errorf("cleanup %s: %w", cs[i].name, err))
		}
	}
	for _, wt := range w.waiters {
		wt.Stop()
	}
	if w.nc != nil {
		w.nc.Close()
	}
	return errors.Join(errs...)
}

// ---- Lazy clients ----

func (w *World) JetStream() (jetstream.JetStream, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.js != nil {
		return w.js, nil
	}
	cfg := w.Suite.Cfg
	opts := []nats.Option{nats.Name("framepulse-acceptance/" + w.Name)}
	if cfg.NATSCreds != "" {
		opts = append(opts, nats.UserCredentials(cfg.NATSCreds))
	}
	nc, err := nats.Connect(cfg.NATSURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect NATS %s: %w", cfg.NATSURL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}
	w.nc, w.js = nc, js
	return js, nil
}

// Events returns (and starts on first use) the event waiter for a video alias.
func (w *World) Events(ctx context.Context, alias string) (*events.Waiter, error) {
	id, err := w.Video(alias)
	if err != nil {
		return nil, err
	}
	js, err := w.JetStream()
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if wt, ok := w.waiters[id]; ok {
		return wt, nil
	}
	wt, err := events.Watch(ctx, js, id)
	if err != nil {
		return nil, err
	}
	w.waiters[id] = wt
	return wt, nil
}

func (w *World) S3() (*s3assert.Store, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.s3 != nil {
		return w.s3, nil
	}
	c := w.Suite.Cfg
	s, err := s3assert.New(c.MinIOEndpoint, c.MinIOAccessKey, c.MinIOSecretKey, c.MinIORegion, c.MinIOUseTLS, c.MaxResponseBytes)
	if err != nil {
		return nil, err
	}
	w.s3 = s
	return s, nil
}

func (w *World) API() (*httpclient.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.api != nil {
		return w.api, nil
	}
	c := w.Suite.Cfg
	api, err := httpclient.New(c.APIBaseURL, c.HTTPTimeout, c.MaxResponseBytes)
	if err != nil {
		return nil, err
	}
	w.api = api
	return api, nil
}

func (w *World) Qdrant() (*qdrant.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.qd != nil {
		return w.qd, nil
	}
	c := w.Suite.Cfg
	q, err := qdrant.New(c.QdrantURL, c.QdrantAPIKey, c.HTTPTimeout)
	if err != nil {
		return nil, err
	}
	w.qd = q
	return q, nil
}

// Latency returns a named recorder for @load scenarios.
func (w *World) Latency(name string) *latency.Recorder {
	w.mu.Lock()
	defer w.mu.Unlock()
	r, ok := w.lat[name]
	if !ok {
		r = latency.NewRecorder(name)
		w.lat[name] = r
	}
	return r
}

// Faults returns the shared injector; RestoreAll is registered as a scenario cleanup the
// first time a scenario asks for it.
func (w *World) Faults() (*fault.Injector, error) {
	inj := w.Suite.Injector
	if inj == nil {
		return nil, errors.New("fault injector not configured")
	}
	w.mu.Lock()
	_, registered := w.Values["__faults_cleanup"]
	w.Values["__faults_cleanup"] = true
	w.mu.Unlock()
	if !registered {
		w.AddCleanup("restore injected faults", inj.RestoreAll)
	}
	return inj, nil
}
