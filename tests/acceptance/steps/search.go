package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

const fSearch = "F6 search"

// searchResultFields are the fields the Feature 6 table may assert (B2 result shape).
var searchResultFields = map[string]bool{
	"video_id": true, "start_ms": true, "end_ms": true, "timestamp": true, "snippet": true,
	"thumbnail_url": true, "playback_url": true, "status": true, "score": true,
}

// Feature 6: Semantic search over processed videos.
func registerSearch(r Registrar) {
	// ---- Background ----
	r.Step(`^chunks (\d+) to (\d+) of video "([^"]+)" are indexed, spanning media time ([\d.]+) s to ([\d.]+) s$`,
		func(ctx context.Context, from, to int, alias string, startS, endS float64) error {
			if to < from || endS <= startS {
				return fmt.Errorf("bad range: chunks %d..%d, %.3f..%.3f s", from, to, startS, endS)
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: seeding through the real pipeline needs a speech fixture whose chunks
			// 42..49 span 85..100 s (8 chunks over 15 s are not ~2 s each - spec gap). Either
			// run the Producer on a long speech fixture, or publish synthetic events with those
			// media_times for pre-cut speech chunks. Finalize V (status "ready").
			return pending(fSearch, "index chunks %d..%d of %s over %.3f..%.3f s", from, to, vid, startS, endS)
		})
	r.Step(`^their transcripts discuss "([^"]+)"$`,
		func(ctx context.Context, topic string) error {
			// TODO: for each chunk, transcript text contains topic words (case-insensitive).
			return pending(fSearch, "transcripts mention %q", topic)
		})
	r.Step(`^their thumbnails exist in MinIO$`,
		func(ctx context.Context) error {
			// TODO: s3.AssertExists(Thumbnail(V, seq), image/jpeg) for 42..49.
			return pending(fSearch, "thumbnails 42..49 exist")
		})

	// ---- Scenario: merged ranges ----
	r.Step(`^the user sends GET "([^"]+)"$`,
		func(ctx context.Context, rawPath string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: LastResponse = api.Get(p); results are global, so filter by video_id V/W -
			// other scenarios' videos may rank higher (isolation gap; use limit=50).
			return pending(fSearch, "GET %s", p)
		})
	r.Step(`^the Serving API embeds the query via "([^"]+)" with purpose (QUERY|PASSAGE)$`,
		func(ctx context.Context, subject, purpose string) error {
			// TODO: core NATS request-reply is not stored; subscribe to the subject (plain sub
			// sees requests too) before the search, decode EmbedRequest, check texts==["architecture"]
			// (unprefixed) and purpose PURPOSE_<purpose>.
			return pending(fSearch, "observe EmbedRequest on %s with purpose %s", subject, purpose)
		})
	r.Step(`^returns (\d+) with a result for video "([^"]+)" containing:$`,
		func(ctx context.Context, status int, alias string, table *godog.Table) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			if table == nil || len(table.Rows) < 2 {
				return fmt.Errorf("expected a Field | Expected Value table")
			}
			if h := table.Rows[0].Cells; len(h) != 2 || h[0].Value != "Field" || h[1].Value != "Expected Value" {
				return fmt.Errorf("table header must be | Field | Expected Value |")
			}
			var fields []string
			for _, row := range table.Rows[1:] {
				f := strings.TrimSpace(row.Cells[0].Value)
				if !searchResultFields[f] {
					return fmt.Errorf("unknown search result field %q", f)
				}
				fields = append(fields, f)
			}
			// TODO: per field: exact match (start_ms, end_ms, timestamp via
			// contracts.FormatTimestampRange, status), "contains" (snippet), pattern
			// (thumbnail_url seq in 42..49; playback_url master + start-time hint - format undefined).
			return pending(fSearch, "status %d, result for %s with fields %v", status, vid, fields)
		})
	r.Step(`^adjacent matching chunks of the same video appear as one range, not (\d+) results$`,
		func(ctx context.Context, n int) error {
			// TODO: exactly one result with video_id V; start/end cover 42..49.
			return pending(fSearch, "one merged range for V instead of %d results", n)
		})

	// ---- Scenario: processing status ----
	r.Step(`^chunks of video "([^"]+)" are indexed but video "([^"]+)" is not finalized yet$`,
		func(ctx context.Context, a, b string) error {
			if a != b {
				return fmt.Errorf("step refers to two videos %q and %q", a, b)
			}
			vid, err := videoArg(ctx, a)
			if err != nil {
				return err
			}
			// TODO: index speech chunks of W without VideoIngestCompleted; the idle timeout
			// (5 min) bounds how long W stays "processing" - scenario must finish before it.
			return pending(fSearch, "index %s without finalizing", vid)
		})
	r.Step(`^the result for video "([^"]+)" is included with status "([^"]+)"$`,
		func(ctx context.Context, alias, status string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			return pending(fSearch, "result for %s has status %s", vid, status)
		})
	r.Step(`^its playback_url responds (\d+) until the video is finalized$`,
		func(ctx context.Context, status int) error {
			// TODO: GET playback_url -> 409; then publish completion, WatchKey finalized, GET -> 200.
			return pending(fSearch, "playback_url %d before finalization, 200 after", status)
		})

	// ---- Scenario: thumbnail redirect ----
	r.Step(`^the client requests the thumbnail_url of the result$`,
		func(ctx context.Context) error {
			// TODO: scenario has no search step - run GET /api/v1/search?q=architecture first
			// (spec gap), then GET result.thumbnail_url without following redirects.
			return pending(fSearch, "search, then GET thumbnail_url")
		})
	r.Step(`^the API responds (\d+) with a Location on the public MinIO endpoint$`,
		func(ctx context.Context, status int) error {
			// TODO: ExpectStatus(302); Location host == MinIOPublicEndpoint host.
			return pending(fSearch, "%d with Location on %s", status, w(ctx).Suite.Cfg.MinIOPublicEndpoint)
		})
	r.Step(`^the presigned URL expires within (\d+) minutes$`,
		func(ctx context.Context, minutes int) error {
			// TODO: httpclient.CheckPresignedURL(MaxTTL minutes); optionally GET it (200 image/jpeg).
			return pending(fSearch, "presigned TTL <= %d min", minutes)
		})

	// ---- Scenario: placeholder ----
	r.Step(`^the thumbnail for the best-matching chunk does not exist yet$`,
		func(ctx context.Context) error {
			// TODO: search, take result thumbnail key, s3.Remove it (the Background created it).
			return pending(fSearch, "remove the best match's thumbnail")
		})
	r.Step(`^the client requests its thumbnail_url$`,
		func(ctx context.Context) error {
			return pending(fSearch, "GET thumbnail_url")
		})
	r.Step(`^the API responds with a placeholder image instead of an error$`,
		func(ctx context.Context) error {
			// TODO: spec gap - status not defined (200 image vs 302 to a placeholder object).
			// Accept 200 image/* or 302 to a presigned placeholder; never 4xx/5xx.
			return pending(fSearch, "placeholder image returned")
		})

	// ---- Scenario: legacy path ----
	r.Step(`^the response is identical to GET "([^"]+)"$`,
		func(ctx context.Context, rawPath string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: same status and JSON-equal body (scores may differ in the last ulp - compare
			// as JSON with float tolerance).
			return pending(fSearch, "response identical to GET %s", p)
		})
}
