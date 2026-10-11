package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

const fFail = "F4 failures"

// Feature 4: Failure handling on both paths.
func registerFailures(r Registrar) {
	r.Step(`^Qdrant is unavailable$`,
		func(ctx context.Context) error {
			// TODO: inj := w.Faults(); inj.ScaleDown(ParseWorkload(Cfg.WorkloadQdrant)).
			// Restored automatically on scenario end.
			return pending(fFail, "scale %s to 0", w(ctx).Suite.Cfg.WorkloadQdrant)
		})
	r.Step(`^a "VideoChunkUploaded" event for chunk (\d+) of video "([^"]+)" is published$`,
		func(ctx context.Context, chunk int, alias string) error {
			seq, err := seqArg(chunk)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload a valid speech chunk + publish (msg-id uploaded:V:seq6).
			return pending(fFail, "publish %s", contracts.MsgIDUploaded(vid, seq))
		})
	r.Step(`^the AI Worker NAKs deliveries (\d+) to (\d+) with increasing backoff delays$`,
		func(ctx context.Context, from, to int) error {
			if from != 1 || to != 4 {
				return fmt.Errorf("deliveries %d..%d: the retry rule is NAK on attempts 1..4 (spec 2.2, A19)", from, to)
			}
			// TODO: NAKs are not stored events. Poll events.ConsumerSnapshot(VIDEO_EVENTS,
			// ai-indexer) and record when DeliveredTotal advances for this message (quiet
			// consumer: scenarios run serially). Expect deliveries 2..5 to follow 1..4 after
			// gaps that grow, ~2 s, 10 s, 30 s, 60 s (B1 AI backoff) within tolerance.
			return pending(fFail, "observe NAK gaps after deliveries %d..%d growing (2s,10s,30s,60s)", from, to)
		})
	r.Step(`^when delivery (\d+) also fails it publishes a "DeadLetter" on "([^"]+)" with "([^"]+)" and delivery_count (\d+)$`,
		func(ctx context.Context, n int, rawSubject, class string, count int) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(class, "ERROR_CLASS_") {
				return fmt.Errorf("unknown error class %q", class)
			}
			if n != 5 || count != 5 {
				return fmt.Errorf("delivery %d / delivery_count %d: attempt 5 dead-letters with delivery_count 5 (A19)", n, count)
			}
			// TODO: WaitFor(OnSubject(subj), WithSchema(DeadLetter)) within RetryTimeout;
			// DecodeDeadLetter: error_class == class, delivery_count == count exactly (not 6:
			// a 6 means the worker NAKed attempt 5 instead of dead-lettering it), consumer
			// ai-indexer, Msg-Id dlq:ai-indexer:{original_stream_seq}.
			return pending(fFail, "DeadLetter on %s with %s and delivery_count %d after delivery %d", subj, class, count, n)
		})
	r.Step(`^terminates the message without a (\d+)(?:st|nd|rd|th) delivery$`,
		func(ctx context.Context, n int) error {
			if n != 6 {
				return fmt.Errorf("no %dth delivery: the rule is that delivery 6 never happens after a Term on attempt 5", n)
			}
			// TODO: events.WaitAckFloor(VIDEO_EVENTS, ai-indexer, original_stream_seq) right
			// after the DeadLetter (Term advances the floor). Then snapshot DeliveredTotal and
			// wait one ack_wait (60 s) + the last backoff: it must not advance for this
			// message, and no second DeadLetter for it may appear (a 6th delivery would be
			// dead-lettered on receipt with delivery_count 6; its Msg-Id dlq:ai-indexer:{seq}
			// is deduplicated, so check the worker log/metric too, not only the stream).
			return pending(fFail, "message terminated; no delivery %d", n)
		})
	r.Step(`^the dead letter contains the original subject, headers and payload verbatim$`,
		func(ctx context.Context) error {
			// TODO: DeadLetter.original_subject/headers/payload equal the published message
			// (fetch it from VIDEO_EVENTS by original_stream_seq with GetMsg and compare bytes).
			// Headers include Nats-Msg-Id, Af-Schema and, when the scenario published with a
			// fault hook, Af-Test-Fault verbatim (B1: original_headers keep it).
			return pending(fFail, "DeadLetter carries the original message verbatim")
		})
	r.Step(`^the Stream Worker still produces "ChunkHlsReady" for chunk (\d+)$`,
		func(ctx context.Context, chunk int) error {
			// TODO: WaitFor(hls-ready:V:src:seq6) within EventTimeout while Qdrant is down.
			return pending(fFail, "ChunkHlsReady for chunk %d despite Qdrant outage", chunk)
		})

	r.Step(`^a message that makes the AI Worker process exit before acking$`,
		func(ctx context.Context) error {
			fv, err := contracts.TestFaultValue(contracts.FaultExitOnReceive, contracts.ConsumerAI, 0)
			if err != nil {
				return err
			}
			// TODO (spec 2.5 fault hook, needs AF_TEST_FAULTS=1 from values-kind.yaml):
			//  1. inj.DeploymentEnv(AI worker, "AF_TEST_FAULTS") == "1", else fail clearly;
			//  2. inj.VerifyHealthyOnRestore(AI worker) so the After hook waits for a healthy
			//     rollout even if the scenario fails mid-loop; pin KEDA (PauseKEDA) at the
			//     current replica count so restarts do not trigger scaling;
			//  3. snapshot inj.PodRestarts(AI worker);
			//  4. upload a valid chunk and events.PublishWithFault(..., fv) on
			//     video.chunks.uploaded.V. The Stream Worker ignores the fault (other consumer).
			return pending(fFail, "publish chunk with %s: %q", contracts.HeaderTestFault, fv)
		})
	r.Step(`^it has been delivered (\d+) times and the worker exited each time$`,
		func(ctx context.Context, n int) error {
			if n != 5 {
				return fmt.Errorf("%d deliveries: the cap allows exactly 5 processing attempts (A19)", n)
			}
			// TODO: wait (RetryTimeout >= 5 x (ack_wait 60 s + restart), ~6 min) until
			// PodRestarts - snapshot == n with LastExitCodes[70] (hook exit code) and
			// ConsumerSnapshot shows the message redelivered n-1 times; no
			// ChunkTranscriptIndexed for the chunk.
			return pending(fFail, "%d crashed deliveries (5 container restarts, exit code 70)", n)
		})
	r.Step(`^the worker that receives the (\d+)(?:st|nd|rd|th) delivery sends it to the DLQ before processing, with delivery_count (\d+), and terminates it$`,
		func(ctx context.Context, n, count int) error {
			if n != 6 || count != 6 {
				return fmt.Errorf("delivery %d / delivery_count %d: want 6 / 6 (dead-lettered on receipt, A19)", n, count)
			}
			// TODO: DeadLetter on dlq.ai-indexer.V: ERROR_CLASS_RETRIES_EXHAUSTED,
			// delivery_count == 6, original_headers still carry Af-Test-Fault verbatim; no
			// 6th restart (PodRestarts unchanged after the DeadLetter: the cap check runs
			// before the hook); WaitAckFloor passes the message; no ChunkTranscriptIndexed.
			return pending(fFail, "DLQ'd on receipt at delivery %d with delivery_count %d, then terminated", n, count)
		})

	r.Step(`^dead letters from consumer "([^"]+)" exist and Qdrant is available again$`,
		func(ctx context.Context, consumer string) error {
			// TODO: produce dead letters for 2 chunks with the fault hook instead of a Qdrant
			// outage: PublishWithFault("fail-transient; consumer=ai-indexer") -> 5 attempts,
			// DLQ RETRIES_EXHAUSTED (~2 min of AI backoff), no scaling needed; Qdrant stays
			// available. Because the fault is unlimited, a redriven message is only indexed if
			// afctl strips Af-Test-Fault (B1), which makes the next steps a strip check too.
			return pending(fFail, "create dead letters for %s, restore Qdrant", consumer)
		})
	r.Step(`^the operator runs "([^"]+)"$`,
		func(ctx context.Context, command string) error {
			args := strings.Fields(command)
			if len(args) == 0 || args[0] != "afctl" {
				return fmt.Errorf("only afctl commands are allowed, got %q", command)
			}
			// TODO: exec Cfg.AfctlBin args[1:] (no shell) with NATS creds; capture output.
			// Scope risk: --since 1h redrives other scenarios' dead letters too; add a
			// --video-id filter to afctl or run this scenario in isolation.
			return pending(fFail, "run %v", args)
		})
	r.Step(`^the original payloads are republished to their original subjects$`,
		func(ctx context.Context) error {
			// TODO: WaitFor msg-id "{orig}:redrive:1" on the original subject with identical
			// payload bytes and headers, except Nats-Msg-Id ({orig}:redrive:n) and
			// Af-Test-Fault, which must be absent (redrive strips it, spec 2.5).
			return pending(fFail, "redriven messages on original subjects with Msg-Id {orig}:redrive:n")
		})
	r.Step(`^each affected chunk is eventually indexed exactly once$`,
		func(ctx context.Context) error {
			// TODO: per chunk: exactly one ChunkTranscriptIndexed (dedup by ai-indexed:V:seq6)
			// and exactly one Qdrant point; CountByVideo == affected chunks.
			return pending(fFail, "each redriven chunk indexed exactly once")
		})
}
