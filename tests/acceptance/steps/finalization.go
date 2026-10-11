package steps

import (
	"context"
	"fmt"
	"time"
)

const fFin = "F2 finalization"

// Feature 2: Video finalization.
func registerFinalization(r Registrar) {
	// ---- Scenario: before finalization ----
	r.Step(`^chunks (\d+) and (\d+) of video "([^"]+)" are READY in "HLS_INDEX" and no "VideoIngestCompleted" has been processed$`,
		func(ctx context.Context, a, b int, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload + publish chunks a and b (no completion); WatchKey until both READY.
			return pending(fFin, "make chunks %d,%d of %s READY without completion", a, b, vid)
		})
	r.Step(`^the client sends GET "([^"]+)"$`,
		func(ctx context.Context, rawPath string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: w.LastResponse = api.Get(p).
			return pending(fFin, "GET %s", p)
		})
	r.Step(`^the API responds (\d+) with Content-Type "([^"]+)" and "Cache-Control: ([^"]+)"$`,
		func(ctx context.Context, status int, contentType, cacheControl string) error {
			if err := httpStatusArg(status); err != nil {
				return err
			}
			// TODO: LastResponse.ExpectStatus, ExpectMediaType, ExpectHeader(Cache-Control);
			// also Retry-After: 5 (spec 2.1, not in the Then-step).
			return pending(fFin, "expect %d, %s, Cache-Control %s", status, contentType, cacheControl)
		})
	r.Step(`^the body has status "([^"]+)", segments_ready (\d+), segments_failed (\d+) and expected_chunk_count (null|\d+)$`,
		func(ctx context.Context, status string, ready, failed int, expected string) error {
			// TODO: json.Unmarshal LastResponse.Body into a struct with *int expected_chunk_count;
			// check video_id equals V too; null must be JSON null, not 0 or absent.
			return pending(fFin, "409 body status=%s ready=%d failed=%d expected=%s", status, ready, failed, expected)
		})
	r.Step(`^GET "([^"]+)" responds the same way$`,
		func(ctx context.Context, rawPath string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: GET p; same status, headers (Content-Type, Cache-Control, Retry-After) and
			// JSON body as LastResponse (master uses key {id}.src).
			return pending(fFin, "GET %s matches the previous 409 response", p)
		})
	r.Step(`^GET "([^"]+)" responds (\d+)$`,
		func(ctx context.Context, rawPath string, status int) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			if err := httpStatusArg(status); err != nil {
				return err
			}
			// TODO: api.Get(p).ExpectStatus(status).
			return pending(fFin, "GET %s expect %d", p, status)
		})

	// ---- Scenario: VideoIngestCompleted finalizes ----
	r.Step(`^chunks (\d+) to (\d+) of video "([^"]+)" are READY in "HLS_INDEX"$`,
		func(ctx context.Context, from, to int, alias string) error {
			if to < from {
				return fmt.Errorf("empty chunk range %d..%d", from, to)
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload + publish chunks from..to; WatchKey until all READY.
			return pending(fFin, "make chunks %d..%d of %s READY", from, to, vid)
		})
	r.Step(`^the Producer publishes "VideoIngestCompleted" with chunk_count (\d+) on "([^"]+)"$`,
		func(ctx context.Context, count int, rawSubject string) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			// TODO: Codec.EncodeVideoIngestCompleted + events.Publish(subj, completed:V).
			return pending(fFin, "publish VideoIngestCompleted(%d) on %s", count, subj)
		})
	r.Step(`^the Stream Worker writes "([^"]+)" with (\d+) segments, "([^"]+)" and "([^"]+)"$`,
		func(ctx context.Context, raw string, n int, a, b string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: after finalized, s3.Get(o); ParseMedia; len(Segments)==n; CheckVOD; tags a, b;
			// object Content-Type application/vnd.apple.mpegurl, Cache-Control max-age=86400.
			return pending(fFin, "%s has %d segments with %s and %s", o, n, a, b)
		})
	r.Step(`^writes "([^"]+)" with exactly one variant "([^"]+)"$`,
		func(ctx context.Context, raw, variant string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: hls.ParseMaster; len(Variants)==1 && URI==variant.
			return pending(fFin, "%s has exactly one variant %s", o, variant)
		})
	r.Step(`^only after both objects exist is "HLS_INDEX" key "([^"]+)" marked finalized$`,
		func(ctx context.Context, rawKey string) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			// TODO: ordering is not directly observable. Approach: KV watch from scenario start;
			// on the first revision with finalized=true, immediately stat both playlists (they
			// must exist). Compare MinIO LastModified <= KV entry Created as a secondary check
			// (clock skew between MinIO and NATS - tolerance needed).
			return pending(fFin, "%s finalized only after playlists exist", key)
		})
	r.Step(`^GET "([^"]+)" returns (\d+) with the content of that object, not a redirect$`,
		func(ctx context.Context, rawPath string, status int) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: Get(p); ExpectStatus(200); no Location header; AssertBytesEqual(playlist).
			return pending(fFin, "GET %s returns %d with proxied bytes", p, status)
		})
	r.Step(`^GET "([^"]+)" returns (\d+) and its variant URI resolves to "([^"]+)"$`,
		func(ctx context.Context, rawPath string, status int, rawTarget string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			target, err := apiPathArg(ctx, rawTarget)
			if err != nil {
				return err
			}
			// TODO: ParseMaster; hls.ResolveVariant(response URL, variant) path == target.
			return pending(fFin, "GET %s returns %d; variant resolves to %s", p, status, target)
		})

	// ---- Scenario: completion before last chunk ----
	r.Step(`^chunks (\d+) to (\d+) of video "([^"]+)" are READY and chunk (\d+) is still being processed$`,
		func(ctx context.Context, from, to int, alias string, inFlight int) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: holding chunk 4 "in processing" deterministically is not possible from
			// outside. Practical approach: publish VideoChunkUploaded for chunk 4 only after the
			// completion step (or delay its raw upload). Spec gap - see test plan.
			return pending(fFin, "chunks %d..%d of %s READY, chunk %d held back", from, to, vid, inFlight)
		})
	r.Step(`^no static playlist is written yet$`,
		func(ctx context.Context) error {
			// TODO: wait until HLS_INDEX expected_chunk_count is set (barrier), then
			// s3.AssertAbsent(playlist) and finalized=false.
			return pending(fFin, "playlist absent after completion recorded")
		})
	r.Step(`^GET "([^"]+)" responds (\d+) with expected_chunk_count (\d+)$`,
		func(ctx context.Context, rawPath string, status, expected int) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: Get(p); 409; JSON expected_chunk_count == expected.
			return pending(fFin, "GET %s expect %d with expected_chunk_count %d", p, status, expected)
		})
	r.Step(`^chunk (\d+) becomes READY$`,
		func(ctx context.Context, chunk int) error {
			// TODO: release the held-back chunk (publish its event) and WatchKey until READY.
			return pending(fFin, "chunk %d READY", chunk)
		})
	r.Step(`^the static playlist is written once, with (\d+) segments and "([^"]+)"$`,
		func(ctx context.Context, n int, tag string) error {
			// TODO: "once": no S3 version history (versioning off) - assert via KV: exactly one
			// revision transitions finalized false->true, and playlist bytes are stable.
			return pending(fFin, "playlist written once with %d segments and %s", n, tag)
		})
	r.Step(`^GET "([^"]+)" returns (\d+)$`,
		func(ctx context.Context, rawPath string, status int) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			if err := httpStatusArg(status); err != nil {
				return err
			}
			// TODO: Get(p).ExpectStatus(status).
			return pending(fFin, "GET %s expect %d", p, status)
		})

	// ---- Scenario: idle timeout ----
	r.Step(`^the Stream Worker runs with FINALIZE_IDLE_TIMEOUT "([^"]+)"$`,
		func(ctx context.Context, raw string) error {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			// TODO: read the deployment env (kubectl get deploy -o jsonpath) and require it
			// equals d. Spec gap: 5 m makes this scenario take >5 min; values-kind.yaml should
			// allow a shorter timeout and this step should read the configured value.
			return pending(fFin, "stream worker FINALIZE_IDLE_TIMEOUT == %s", d)
		})
	r.Step(`^chunks (\d+(?:, \d+)* and \d+) of video "([^"]+)" are READY and no "VideoIngestCompleted" has been published$`,
		func(ctx context.Context, list, alias string) error {
			seqs, err := intList(list)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload + publish only these chunks; WatchKey until each READY.
			return pending(fFin, "chunks %v of %s READY, no completion", seqs, vid)
		})
	r.Step(`^no event for video "([^"]+)" arrives for (\d+) minutes$`,
		func(ctx context.Context, alias string, minutes int) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: do nothing; the next Then waits (FinalizeTimeout) on HLS_INDEX finalized.
			// Assert afterwards that the waiter saw no new event for vid in that window.
			return pending(fFin, "idle %d min for %s (no publish)", minutes, vid)
		})
	r.Step(`^the video is finalized with finalize_reason (COMPLETED|IDLE_TIMEOUT)$`,
		func(ctx context.Context, reason string) error {
			// TODO: WatchKey(V.src, finalized && finalize_reason == FINALIZE_REASON_<reason>)
			// within FinalizeTimeout; check last_activity_at + timeout <= finalize time.
			return pending(fFin, "finalized with reason %s", reason)
		})
	r.Step(`^the static playlist lists (.+) with "([^"]+)" before "([^"]+)"$`,
		func(ctx context.Context, list, tag, before string) error {
			uris := quotedList(list)
			if len(uris) == 0 {
				return fmt.Errorf("no segment URIs in %q", list)
			}
			// TODO: ParseMedia; URIs()==uris; Find(before).DiscontinuityBefore; no other
			// discontinuity (hls.ExpectedDiscontinuities).
			return pending(fFin, "playlist lists %v with %s before %s", uris, tag, before)
		})
	r.Step(`^the playlist ends with "([^"]+)"$`,
		func(ctx context.Context, tag string) error {
			// TODO: last non-empty line == tag.
			return pending(fFin, "playlist ends with %s", tag)
		})

	// ---- Scenario: no playable chunk ----
	r.Step(`^chunk (\d+) of video "([^"]+)" is FAILED, no other chunk exists and the idle timeout has passed$`,
		func(ctx context.Context, chunk int, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: publish a corrupted chunk; wait FAILED; then WatchKey finalized (IDLE_TIMEOUT).
			return pending(fFin, "chunk %d of %s FAILED and idle-finalized", chunk, vid)
		})
	r.Step(`^the video is finalized with finalize_reason (IDLE_TIMEOUT) and no playlist object is written$`,
		func(ctx context.Context, reason string) error {
			// TODO: HlsIndex finalized, reason, zero READY; AssertAbsent(media + master playlist).
			return pending(fFin, "finalized %s with no playlist objects", reason)
		})
	r.Step(`^GET "([^"]+)" responds (\d+) with status "([^"]+)"$`,
		func(ctx context.Context, rawPath string, status int, s string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			// TODO: Get(p); 410; JSON {"video_id":V,"status":"failed"}; same for master.m3u8.
			return pending(fFin, "GET %s expect %d with status %q", p, status, s)
		})

	// ---- Scenario: late chunk ----
	r.Step(`^video "([^"]+)" was force-finalized after the idle timeout$`,
		func(ctx context.Context, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: chunks 0..1 READY, no completion, WatchKey finalized IDLE_TIMEOUT; snapshot
			// playlist bytes, KV revision and summed fp_late_chunks_total.
			return pending(fFin, "force-finalize %s and snapshot state", vid)
		})
	r.Step(`^a "VideoChunkUploaded" event for a new chunk of video "([^"]+)" is delivered$`,
		func(ctx context.Context, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: publish chunk max+1; wait for ChunkHlsReady (late chunk is still processed).
			return pending(fFin, "deliver late chunk for %s", vid)
		})
	r.Step(`^the static playlist is unchanged and "HLS_INDEX" key "([^"]+)" stays finalized$`,
		func(ctx context.Context, rawKey string) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			// TODO: AssertBytesEqual(playlist, snapshot); KV revision unchanged and finalized.
			return pending(fFin, "playlist unchanged, %s still finalized", key)
		})
	r.Step(`^metric "([^"]+)" is incremented$`,
		func(ctx context.Context, name string) error {
			// TODO: scrape /metrics of every stream-worker pod (pod IPs via kubectl,
			// Cfg.StreamWorkerMetricsPort), metrics.Sum(name) > snapshot. Pod restarts reset
			// counters - spec gap.
			return pending(fFin, "metric %s incremented across stream-worker replicas", name)
		})
}
