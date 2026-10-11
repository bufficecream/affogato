// Package steps holds the godog step definitions, one file per spec feature.
//
// Status: every step is registered and returns godog.ErrPending with a TODO that names
// what it will verify and which internal helper to use. Arguments are already parsed
// and validated (aliases expanded, object keys / subjects / seq6 checked), so wrong spec
// text fails immediately instead of being reported as pending.
//
// Conventions for implementers:
//   - Resolve aliases ("V", "W") with arg helpers below; never hard-code IDs.
//   - "Then" steps wait via world.Events(ctx, alias).WaitFor(...) or events.WatchKey on
//     HLS_INDEX, bounded by the Cfg timeouts, never time.Sleep.
//   - Protobuf payloads go through w.Suite.Codec; payload.ErrNotImplemented => pendingErr.
//   - Fault injection goes through w.Faults(), which restores on scenario end.
package steps

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
	"github.com/framepulse/framepulse/tests/acceptance/internal/payload"
	"github.com/framepulse/framepulse/tests/acceptance/internal/world"
)

// Registrar is the subset of *godog.ScenarioContext used for registration, so tests can
// record definitions without running godog.
type Registrar interface {
	Step(expr, stepFunc interface{})
}

// RegisterAll registers every step of every feature.
func RegisterAll(r Registrar) {
	registerHotPath(r)
	registerFinalization(r)
	registerColdPath(r)
	registerFailures(r)
	registerIdempotency(r)
	registerSearch(r)
	registerRetention(r)
	registerShared(r)
}

// pending marks a step as not implemented yet. The message names what it will verify.
func pending(feature, format string, args ...any) error {
	return fmt.Errorf("%w: TODO(%s): %s", godog.ErrPending, feature, fmt.Sprintf(format, args...))
}

// pendingErr maps "needs code that does not exist yet" errors to pending.
func pendingErr(feature string, err error) error {
	if errors.Is(err, payload.ErrNotImplemented) {
		return pending(feature, "%v", err)
	}
	return err
}

func w(ctx context.Context) *world.World { return world.From(ctx) }

// ---- argument helpers ----

func videoArg(ctx context.Context, alias string) (string, error) { return w(ctx).Video(alias) }

// objectArg expands aliases in "bucket/key" spec text and validates the result.
func objectArg(ctx context.Context, raw string) (contracts.Object, error) {
	s, err := w(ctx).Expand(raw)
	if err != nil {
		return contracts.Object{}, err
	}
	return contracts.ParseObject(s)
}

var subjectTokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// subjectArg expands aliases in a subject and rejects wildcards and empty tokens.
func subjectArg(ctx context.Context, raw string) (string, error) {
	s, err := w(ctx).Expand(raw)
	if err != nil {
		return "", err
	}
	for _, tok := range strings.Split(s, ".") {
		if !subjectTokenRe.MatchString(tok) {
			return "", fmt.Errorf("subject %q: invalid token %q", s, tok)
		}
	}
	return s, nil
}

// kvKeyArg expands "V.src" into "{video_id}.src".
func kvKeyArg(ctx context.Context, raw string) (string, error) {
	s, err := w(ctx).Expand(raw)
	if err != nil {
		return "", err
	}
	vid, rendition, ok := strings.Cut(s, ".")
	if !ok {
		return "", fmt.Errorf("HLS_INDEX key %q: want {video_id}.{rendition}", s)
	}
	if err := contracts.ValidateVideoID(vid); err != nil {
		return "", err
	}
	if rendition != contracts.RenditionSrc {
		return "", fmt.Errorf("HLS_INDEX key %q: unknown rendition %q", s, rendition)
	}
	return s, nil
}

// apiPathArg expands aliases in an API path; only Serving API routes are allowed.
func apiPathArg(ctx context.Context, raw string) (string, error) {
	s, err := w(ctx).Expand(raw)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(s, "/api/") {
		return "", fmt.Errorf("path %q is not a Serving API route", s)
	}
	return s, nil
}

// pointArg turns `V:000042` into the expected Qdrant point ID.
func pointArg(ctx context.Context, raw string) (uuid.UUID, error) {
	s, err := w(ctx).Expand(raw)
	if err != nil {
		return uuid.Nil, err
	}
	return contracts.PointIDFromName(w(ctx).Suite.Cfg.PointIDNamespace, s)
}

// segmentURIArg validates a playlist segment URI such as "000003.ts" and returns its seq.
func segmentURIArg(uri string) (uint32, error) {
	base, ok := strings.CutSuffix(uri, ".ts")
	if !ok {
		return 0, fmt.Errorf("segment URI %q: want {seq6}.ts", uri)
	}
	return contracts.ParseSeq6(base)
}

var quotedRe = regexp.MustCompile(`"([^"]*)"`)

// quotedList extracts the quoted items from `"a", "b" and "c"`.
func quotedList(s string) []string {
	var out []string
	for _, m := range quotedRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// intList parses "2, 0 and 1" or "0, 1, 2".
func intList(s string) ([]int, error) {
	s = strings.ReplaceAll(s, " and ", ", ")
	var out []int
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, fmt.Errorf("number list %q: %w", s, err)
		}
		out = append(out, n)
	}
	return out, nil
}

func seqArg(n int) (uint32, error) {
	if n < 0 || n > contracts.MaxChunkSeq {
		return 0, fmt.Errorf("chunk_seq %d out of range", n)
	}
	return uint32(n), nil
}

func httpStatusArg(n int) error {
	if n < 100 || n > 599 {
		return fmt.Errorf("HTTP status %d invalid", n)
	}
	return nil
}
