package steps

import (
	"context"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
)

const fIdem = "F5 idempotency"

// Feature 5: Idempotent processing.
func registerIdempotency(r Registrar) {
	r.Step(`^the Producer publishes "VideoChunkUploaded" for chunk (\d+) of video "([^"]+)" twice with the same Nats-Msg-Id$`,
		func(ctx context.Context, chunk int, alias string) error {
			seq, err := seqArg(chunk)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: events.Publish twice with Msg-Id uploaded:V:seq6; second PubAck.Duplicate
			// must be true (verified against an in-process server in internal/events tests).
			return pending(fIdem, "publish %s twice", contracts.MsgIDUploaded(vid, seq))
		})
	r.Step(`^"VIDEO_EVENTS" stores exactly one message for that chunk$`,
		func(ctx context.Context) error {
			// TODO: events.StoredCount(VIDEO_EVENTS, subject) == 1 (subject holds only this
			// video's uploads, which is one chunk in this scenario).
			return pending(fIdem, "StoredCount(video.chunks.uploaded.V) == 1")
		})

	// ---- Scenario Outline: redelivery after crash before ack ----
	r.Step(`^chunk (\d+) of video "([^"]+)" has been fully processed by (Stream Worker|AI Worker) but not acked$`,
		func(ctx context.Context, chunk int, alias, worker string) error {
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: spec gap - "processed but not acked" needs a hook (e.g. env
			// FP_TEST_SKIP_ACK_FOR_SOURCE_ID in values-kind.yaml) or killing the pod between
			// PubAck and ack, which is racy. Snapshot outputs (object bytes, KV, point) here.
			return pending(fIdem, "%s processed chunk %d of %s without ack", worker, chunk, vid)
		})
	r.Step(`^the event is redelivered to (Stream Worker|AI Worker)$`,
		func(ctx context.Context, worker string) error {
			// TODO: wait until consumer redelivers (ack_wait 15 s / 60 s) and the worker acks.
			return pending(fIdem, "redelivery to %s observed", worker)
		})
	r.Step(`^the same object keys are overwritten with equivalent content$`,
		func(ctx context.Context) error {
			// TODO: segment/thumbnail (hot) or transcript (cold) exist at the same keys and are
			// equivalent: byte-equal for segments; transcript JSON equal modulo timestamps.
			// No extra objects under the video prefix.
			return pending(fIdem, "same keys, equivalent content, no extra objects")
		})
	r.Step(`^"HLS_INDEX" key "([^"]+)" lists chunk (\d+) exactly once$`,
		func(ctx context.Context, rawKey string, chunk int) error {
			key, err := kvKeyArg(ctx, rawKey)
			if err != nil {
				return err
			}
			// TODO: DecodeHlsIndex; CountSeq(chunk) == 1.
			return pending(fIdem, "%s lists chunk %d once", key, chunk)
		})
	r.Step(`^Qdrant holds exactly one point with id UUIDv5\("([^"]+)"\)$`,
		func(ctx context.Context, name string) error {
			id, err := pointArg(ctx, name)
			if err != nil {
				return err
			}
			// TODO: GetPoint(transcripts, id) found and CountByVideo(V) == 1.
			return pending(fIdem, "exactly one Qdrant point %s", id)
		})
	r.Step(`^the result event is deduplicated, so "VIDEO_EVENTS" holds exactly one (ChunkHlsReady|ChunkTranscriptIndexed) for chunk (\d+)$`,
		func(ctx context.Context, event string, chunk int) error {
			// TODO: count waiter events WithMsgID(hls-ready:V:src:seq6 / ai-indexed:V:seq6) == 1
			// after the redelivery's ack (redelivery publish must get PubAck.Duplicate).
			return pending(fIdem, "exactly one %s for chunk %d", event, chunk)
		})
}
