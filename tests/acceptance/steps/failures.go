package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
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
	r.Step(`^the AI Worker NAKs it with increasing backoff delays$`,
		func(ctx context.Context) error {
			// TODO: NAKs are not stored events. Observe redeliveries: poll consumer info
			// (num_redelivered / delivered.consumer_seq) or the worker metric for retries, and
			// check gaps grow (2 s, 10 s, 30 s, 60 s per B1) within tolerance.
			return pending(fFail, "observe redelivery gaps growing (2s,10s,30s,60s)")
		})
	r.Step(`^after the (\d+)(?:st|nd|rd|th) delivery publishes a "DeadLetter" on "([^"]+)" with "([^"]+)"$`,
		func(ctx context.Context, n int, rawSubject, class string) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(class, "ERROR_CLASS_") {
				return fmt.Errorf("unknown error class %q", class)
			}
			// TODO: WaitFor(subj) within RetryTimeout; DeadLetter.error_class == class,
			// delivery_count == n (spec says "after the 5th delivery"; B1 says on receipt
			// when num_delivered > 5, i.e. the 6th - ambiguity, see test plan).
			return pending(fFail, "DeadLetter on %s with %s after delivery %d", subj, class, n)
		})
	r.Step(`^the dead letter contains the original subject, headers and payload verbatim$`,
		func(ctx context.Context) error {
			// TODO: DeadLetter.original_subject/headers/payload equal the published message
			// (fetch it from VIDEO_EVENTS by original_stream_seq with GetMsg and compare bytes).
			return pending(fFail, "DeadLetter carries the original message verbatim")
		})
	r.Step(`^the Stream Worker still produces "ChunkHlsReady" for chunk (\d+)$`,
		func(ctx context.Context, chunk int) error {
			// TODO: WaitFor(hls-ready:V:src:seq6) within EventTimeout while Qdrant is down.
			return pending(fFail, "ChunkHlsReady for chunk %d despite Qdrant outage", chunk)
		})

	r.Step(`^a message that makes the AI Worker process exit before acking$`,
		func(ctx context.Context) error {
			// TODO: spec gap - no defined poison input crashes the worker. Options: a debug
			// hook in values-kind.yaml (e.g. FP_TEST_CRASH_ON_SOURCE_ID=crash-me, crash when
			// VideoChunkUploaded.source_id matches), or an OOM-sized input. Needs developer input.
			return pending(fFail, "publish a crash-inducing message (needs a test hook)")
		})
	r.Step(`^it has been delivered more than (\d+) times$`,
		func(ctx context.Context, n int) error {
			// TODO: watch consumer info until the message's num_delivered > n (pod restarts
			// visible via kubectl restartCount).
			return pending(fFail, "message delivered more than %d times", n)
		})
	r.Step(`^the next worker to receive it sends it to the DLQ before processing and terminates it$`,
		func(ctx context.Context) error {
			// TODO: DeadLetter on dlq.ai-indexer.V with ERROR_CLASS_RETRIES_EXHAUSTED and
			// delivery_count 6; no ChunkTranscriptIndexed; no further redelivery.
			return pending(fFail, "DLQ'd on receipt with RETRIES_EXHAUSTED, then terminated")
		})

	r.Step(`^dead letters from consumer "([^"]+)" exist and Qdrant is available again$`,
		func(ctx context.Context, consumer string) error {
			// TODO: run the Qdrant-outage flow for 2+ chunks until DLQ'd, then RestoreAll and
			// wait for Qdrant ready.
			return pending(fFail, "create dead letters for %s, restore Qdrant", consumer)
		})
	r.Step(`^the operator runs "([^"]+)"$`,
		func(ctx context.Context, command string) error {
			args := strings.Fields(command)
			if len(args) == 0 || args[0] != "fpctl" {
				return fmt.Errorf("only fpctl commands are allowed, got %q", command)
			}
			// TODO: exec Cfg.FpctlBin args[1:] (no shell) with NATS creds; capture output.
			// Scope risk: --since 1h redrives other scenarios' dead letters too; add a
			// --video-id filter to fpctl or run this scenario in isolation.
			return pending(fFail, "run %v", args)
		})
	r.Step(`^the original payloads are republished to their original subjects$`,
		func(ctx context.Context) error {
			// TODO: WaitFor msg-id "{orig}:redrive:1" on the original subject with identical
			// payload bytes and headers (except Nats-Msg-Id).
			return pending(fFail, "redriven messages on original subjects with Msg-Id {orig}:redrive:n")
		})
	r.Step(`^each affected chunk is eventually indexed exactly once$`,
		func(ctx context.Context) error {
			// TODO: per chunk: exactly one ChunkTranscriptIndexed (dedup by ai-indexed:V:seq6)
			// and exactly one Qdrant point; CountByVideo == affected chunks.
			return pending(fFail, "each redriven chunk indexed exactly once")
		})
}
