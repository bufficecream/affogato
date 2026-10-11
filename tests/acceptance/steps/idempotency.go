package steps

import (
	"context"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
)

const fIdem = "F5 idempotency"

// workerConsumer maps the worker names used in Feature 5 to their durable consumers.
func workerConsumer(worker string) string {
	if worker == "AI Worker" {
		return contracts.ConsumerAI
	}
	return contracts.ConsumerHls
}

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
			seq, err := seqArg(chunk)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			consumer := workerConsumer(worker)
			fv, err := contracts.TestFaultValue(contracts.FaultCrashBeforeAck, consumer, 0)
			if err != nil {
				return err
			}
			// TODO (spec 2.5 fault hook; default n = 1, so delivery 2 is processed and acked):
			//  1. require FP_TEST_FAULTS=1 on the worker (inj.DeploymentEnv) and register
			//     inj.VerifyHealthyOnRestore(worker) + PodRestarts snapshot;
			//  2. upload the chunk, PublishWithFault(uploaded:V:seq6, fv) and keep the PubAck
			//     stream seq;
			//  3. wait for the first result event (hls-ready:V:src:seq6 or ai-indexed:V:seq6):
			//     it is published before the crash;
			//  4. snapshot outputs: segment + thumbnail bytes and HLS_INDEX revision (hot), or
			//     transcript JSON and the Qdrant point (cold).
			// Stream Worker in kind: the redelivery comes after ack_wait (15 s) + restart, so
			// with FINALIZE_IDLE_TIMEOUT=10s V may be idle-finalized first and the redelivered
			// chunk counts as late. Do not assert V is unfinalized; the steps below hold either way.
			return pending(fIdem, "%s processed %s of %s, crashed before ack (%s: %q)", worker,
				contracts.MsgIDUploaded(vid, seq), vid, contracts.HeaderTestFault, fv)
		})
	r.Step(`^the event is redelivered to (Stream Worker|AI Worker)$`,
		func(ctx context.Context, worker string) error {
			// TODO: wait ack_wait (15 s hot / 60 s AI) + container restart: PodRestarts grows by
			// exactly 1 with exit code 70; then events.WaitAckFloor(VIDEO_EVENTS,
			// workerConsumer(worker), stream seq of the upload) until the floor passes the message (delivery 2
			// processed and acked). Bound: ack_wait + RolloutTimeout.
			return pending(fIdem, "redelivery to %s (%s) observed and acked", worker, workerConsumer(worker))
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
