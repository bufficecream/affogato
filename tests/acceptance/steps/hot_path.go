package steps

import (
	"context"
	"fmt"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

const fHot = "F1 hot-path"

// Feature 1: Chunk processing into HLS segments.
func registerHotPath(r Registrar) {
	// ---- Background ----
	r.Step(`^the Affogato Helm chart is deployed to the kind test cluster and all pods are ready$`,
		func(ctx context.Context) error {
			// TODO: w.Faults().WaitReady(ctx); verify streams VIDEO_EVENTS/VIDEO_DLQ, KV
			// HLS_INDEX and alias "transcripts" exist (fail fast with a clear message).
			return pending(fHot, "wait for deployments Available / pods Ready in %s", w(ctx).Suite.Cfg.KubeNamespace)
		})
	r.Step(`^a new video_id "([^"]+)" has been assigned by the Producer CLI$`,
		func(ctx context.Context, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: spec gap - the Producer assigns video_id; the test needs the CLI to accept
			// --video-id (or to print the assigned one). Start the event waiter here so no
			// event can be missed: w.Events(ctx, alias).
			return pending(fHot, "attach event waiter for %s=%s", alias, vid)
		})

	// ---- Scenario: valid video ----
	r.Step(`^the Producer CLI ingests "([^"]+)" \(H\.264 \+ AAC\) as video "([^"]+)"$`,
		func(ctx context.Context, fixture, alias string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: exec Cfg.ProducerBin with FixturesDir/fixture and --video-id vid; record the
			// publish time of each chunk for latency; register cleanup of af-raw/af-media prefixes.
			return pending(fHot, "run Producer CLI on %s for video %s", fixture, vid)
		})
	r.Step(`^the raw object "([^"]+)" exists with content-type "([^"]+)"$`,
		func(ctx context.Context, raw, contentType string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: s3.AssertExists(o, contentType); also x-amz-meta-af-sha256 == sha256(body).
			return pending(fHot, "assert %s exists with content-type %s", o, contentType)
		})
	r.Step(`^an? "([^"]+)" event for chunk (\d+) is published on "([^"]+)"$`,
		func(ctx context.Context, event string, chunk int, rawSubject string) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			if _, err := seqArg(chunk); err != nil {
				return err
			}
			// TODO: WaitFor(OnSubject(subj) && WithMsgID(uploaded:/hls-ready:…)) and check
			// Af-Schema + Content-Type headers; decode and check chunk_seq.
			return pending(fHot, "wait for %s chunk %d on %s", event, chunk, subj)
		})
	r.Step(`^the Stream Worker writes "([^"]+)" by remuxing, without re-encoding$`,
		func(ctx context.Context, raw string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: after ChunkHlsReady, compare `ffprobe -show_packets` (or ffmpeg -c copy
			// -f framemd5) of the raw chunk and the segment: same codec params, same packet
			// count and payload hashes. Requires ffprobe in the runner image.
			return pending(fHot, "verify %s is a remux of the raw chunk (packet hashes equal)", o)
		})
	r.Step(`^writes the thumbnail "([^"]+)"$`,
		func(ctx context.Context, raw string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: s3.AssertExists(o, image/jpeg); decode JPEG header, expect 320x180 (B2).
			return pending(fHot, "assert thumbnail %s exists as image/jpeg", o)
		})
	r.Step(`^"HLS_INDEX" key "([^"]+)" lists chunk (\d+) as READY with its measured duration$`,
		func(ctx context.Context, rawKey string, chunk int) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			// TODO: get KV entry, Codec.DecodeHlsIndex; segment chunk READY with media_time
			// equal to ChunkHlsReady.media_time (not an assumed 2000 ms). Store the duration
			// in w.Values for the EXTINF step.
			return pending(fHot, "assert %s chunk %d READY with measured duration", key, chunk)
		})
	r.Step(`^"HLS_INDEX" key "([^"]+)" is marked finalized$`,
		func(ctx context.Context, rawKey string) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			// TODO: events.WatchKey(HLS_INDEX, key, finalized==true) within EventTimeout.
			return pending(fHot, "watch %s until finalized=true", key)
		})
	r.Step(`^GET "([^"]+)" returns (\d+) with "([^"]+)" and "([^"]+)"$`,
		func(ctx context.Context, rawPath string, status int, a, b string) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			if err := httpStatusArg(status); err != nil {
				return err
			}
			// TODO: api.Get(p); ExpectStatus; ExpectMediaType(application/vnd.apple.mpegurl);
			// hls.ParseMedia + CheckVOD; body contains a and b. Keep in w.LastResponse.
			return pending(fHot, "GET %s expect %d containing %q and %q", p, status, a, b)
		})
	r.Step(`^the response body is byte-identical to the object "([^"]+)"$`,
		func(ctx context.Context, raw string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: s3.AssertBytesEqual(o, w.LastResponse.Body).
			return pending(fHot, "compare last response body with %s", o)
		})
	r.Step(`^the playlist references "([^"]+)" with an EXTINF equal to the measured duration$`,
		func(ctx context.Context, uri string) error {
			if _, err := segmentURIArg(uri); err != nil {
				return err
			}
			// TODO: hls.ParseMedia(LastResponse.Body).Find(uri).Duration == measured duration
			// (ms precision; define rounding - spec gap).
			return pending(fHot, "EXTINF of %s equals measured duration", uri)
		})
	r.Step(`^GET "([^"]+)" returns (\d+) to a presigned MinIO URL$`,
		func(ctx context.Context, rawPath string, status int) error {
			p, err := apiPathArg(ctx, rawPath)
			if err != nil {
				return err
			}
			if err := httpStatusArg(status); err != nil {
				return err
			}
			// TODO: api.Get(p) (no redirect follow); Location() + httpclient.CheckPresignedURL
			// against MinIOPublicEndpoint and the af-media segment key; MaxTTL 10m.
			return pending(fHot, "GET %s expect %d with presigned Location", p, status)
		})

	// ---- @load Scenario ----
	r.Step(`^the Producer CLI publishes (\d+) chunks at real-time rate \(one per (\d+) s per video, (\d+) videos\)$`,
		func(ctx context.Context, chunks, perSeconds, videos int) error {
			if videos <= 0 || chunks%videos != 0 {
				return fmt.Errorf("%d chunks do not split evenly over %d videos", chunks, videos)
			}
			// TODO: run `videos` Producer CLI processes in parallel on a long fixture paced at
			// real time (60 chunks each); record stream timestamps of uploaded events and of
			// HLS_INDEX READY transitions in w.Latency("publish-to-ready").
			return pending(fHot, "publish %d chunks (%d videos, 1 per %d s each)", chunks, videos, perSeconds)
		})
	r.Step(`^p99 publish-to-delivery latency for consumer "([^"]+)" is at most (\d+) ms$`,
		func(ctx context.Context, consumer string, ms int) error {
			if consumer != contracts.ConsumerHls && consumer != contracts.ConsumerAI {
				return fmt.Errorf("unknown consumer %q", consumer)
			}
			// TODO: delivery time is only observable inside the worker: scrape histogram
			// af_consume_latency_seconds{consumer} from all replicas (metrics.Sum on buckets),
			// compute p99 from bucket deltas over the run window.
			return pending(fHot, "p99 delivery latency for %s <= %d ms (from worker histogram)", consumer, ms)
		})
	r.Step(`^p95 publish-to-ready latency \(segment READY in HLS_INDEX\) is at most ([\d.]+) s$`,
		func(ctx context.Context, seconds float64) error {
			// TODO: w.Latency("publish-to-ready").AssertAtMost(95, seconds, 300).
			return pending(fHot, "p95 publish-to-ready <= %.1f s", seconds)
		})

	// ---- Scenario: out-of-order ----
	r.Step(`^chunks (\d+(?:, \d+)* and \d+) of video "([^"]+)" are processed in that order by different replicas$`,
		func(ctx context.Context, list, alias string) error {
			seqs, err := intList(list)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload raw chunks, publish VideoChunkUploaded in the given order, each after
			// the previous ChunkHlsReady. "Different replicas" needs >= 2 stream-worker replicas
			// and evidence (Af-Producer / worker_instance) - spec gap: not observable on success.
			return pending(fHot, "publish chunks %v of %s in order", seqs, vid)
		})
	r.Step(`^"HLS_INDEX" key "([^"]+)" lists chunks (\d+(?:, \d+)+) in order exactly once each$`,
		func(ctx context.Context, rawKey, list string) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			seqs, err := intList(list)
			if err != nil {
				return err
			}
			// TODO: decode HlsIndex; SegmentsInOrder() and CountSeq(s)==1 for each.
			return pending(fHot, "assert %s lists %v in order, once each", key, seqs)
		})
	r.Step(`^the static playlist "([^"]+)" lists ((?:"[^"]+"(?:, )?)+) in order$`,
		func(ctx context.Context, raw, list string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			uris := quotedList(list)
			for _, u := range uris {
				if _, err := segmentURIArg(u); err != nil {
					return err
				}
			}
			// TODO: s3.Get(o); hls.ParseMedia; URIs() == uris; CheckVOD().
			return pending(fHot, "assert %s lists %v in order", o, uris)
		})

	// ---- Scenario: corrupted chunk ----
	r.Step(`^video "([^"]+)" has (\d+) chunks and the raw chunk object for chunk (\d+) contains invalid media bytes$`,
		func(ctx context.Context, alias string, n, bad int) error {
			if bad >= n {
				return fmt.Errorf("corrupted chunk %d outside 0..%d", bad, n-1)
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload fixture chunks 0..n-1 to af-raw (chunk_corrupted.ts for `bad`) with
			// the sha256 of the bytes actually uploaded, so the failure is INVALID_MEDIA and
			// not CHECKSUM_MISMATCH.
			return pending(fHot, "upload %d raw chunks for %s with chunk %d corrupted", n, vid, bad)
		})
	r.Step(`^the "VideoChunkUploaded" events for chunks (\d+) to (\d+) and "VideoIngestCompleted" with chunk_count (\d+) are published$`,
		func(ctx context.Context, from, to, count int) error {
			if to < from || count != to-from+1 {
				return fmt.Errorf("chunks %d..%d inconsistent with chunk_count %d", from, to, count)
			}
			// TODO: encode + events.Publish each VideoChunkUploaded, then VideoIngestCompleted.
			return pending(fHot, "publish uploaded events %d..%d and completed(%d)", from, to, count)
		})
	r.Step(`^the Stream Worker's ffprobe process fails on chunk (\d+) and is classified as "([^"]+)"$`,
		func(ctx context.Context, chunk int, class string) error {
			// TODO: wait for DLQ event; DecodeDeadLetter: error_class == class, chunk == chunk,
			// error_code non-empty (e.g. FFPROBE_*). "ffprobe process fails" itself is only
			// visible in logs - assert via error_code.
			return pending(fHot, "DeadLetter for chunk %d has error_class %s", chunk, class)
		})
	r.Step(`^a "DeadLetter" is published on "([^"]+)" with redrivable = (true|false)$`,
		func(ctx context.Context, rawSubject, redrivable string) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			// TODO: WaitFor(OnSubject(subj) && WithSchema(DeadLetter)); Msg-Id dlq:{consumer}:{seq}.
			return pending(fHot, "wait for DeadLetter on %s redrivable=%s", subj, redrivable)
		})
	r.Step(`^the original message is terminated, not redelivered$`,
		func(ctx context.Context) error {
			// TODO: DeadLetter.delivery_count == 1, and consumer info for hls-stream-worker
			// shows no pending/redelivered message for that stream seq (ack floor passed it).
			return pending(fHot, "assert original message was Term()'d (delivery_count 1, ack floor advanced)")
		})
	r.Step(`^chunk (\d+) is recorded as FAILED in "HLS_INDEX"$`,
		func(ctx context.Context, chunk int) error {
			// TODO: WatchKey(V.src) until segment chunk status FAILED.
			return pending(fHot, "HLS_INDEX segment %d FAILED", chunk)
		})
	r.Step(`^the Stream Worker remains healthy and processes chunks (\d+) and (\d+) normally$`,
		func(ctx context.Context, a, b int) error {
			// TODO: ChunkHlsReady for a and b; stream-worker pods show no restarts since the
			// scenario began (kubectl get pods restartCount).
			return pending(fHot, "ChunkHlsReady for chunks %d and %d, no worker restarts", a, b)
		})
	r.Step(`^the video is finalized with a static playlist that omits "([^"]+)"$`,
		func(ctx context.Context, uri string) error {
			if _, err := segmentURIArg(uri); err != nil {
				return err
			}
			// TODO: WatchKey finalized; playlist from MinIO; Find(uri) absent; CheckVOD.
			return pending(fHot, "finalized playlist omits %s", uri)
		})
	r.Step(`^that playlist has "([^"]+)" immediately before "([^"]+)"$`,
		func(ctx context.Context, tag, uri string) error {
			if _, err := segmentURIArg(uri); err != nil {
				return err
			}
			// TODO: Find(uri).DiscontinuityBefore (tag must be #EXT-X-DISCONTINUITY).
			return pending(fHot, "%s immediately before %s", tag, uri)
		})
}
