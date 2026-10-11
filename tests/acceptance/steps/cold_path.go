package steps

import (
	"context"
	"fmt"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

const fCold = "F3 cold-path"

// Feature 3: Asynchronous transcription and vector indexing.
func registerColdPath(r Registrar) {
	r.Step(`^the AI Worker runs with the default WHISPER_MODEL "([^"]+)"( \(int8, CPU\))?$`,
		func(ctx context.Context, model, _ string) error {
			switch model {
			case "tiny", "base", "small", "medium", "large-v3":
			default:
				return fmt.Errorf("WHISPER_MODEL %q not in the allowed list (spec 2.3)", model)
			}
			// TODO: kubectl get the ai-worker deployment env: WHISPER_MODEL unset or == model;
			// no GPU resources requested.
			return pending(fCold, "ai-worker WHISPER_MODEL == %s on CPU", model)
		})
	r.Step(`^chunk (\d+) of video "([^"]+)" contains English speech about "([^"]+)"$`,
		func(ctx context.Context, chunk int, alias, topic string) error {
			seq, err := seqArg(chunk)
			if err != nil {
				return err
			}
			vid, err := videoArg(ctx, alias)
			if err != nil {
				return err
			}
			// TODO: upload FixturesDir/chunk_speech_en.ts as raw chunk seq for vid with sha256;
			// the fixture must be provided (tests/fixtures/README.md). Skip with a clear error
			// if missing.
			return pending(fCold, "upload English speech chunk %s (%q) for %s", contracts.Seq6(seq), topic, vid)
		})
	r.Step(`^its "([^"]+)" event is delivered to consumer "([^"]+)"$`,
		func(ctx context.Context, event, consumer string) error {
			if consumer != contracts.ConsumerAI {
				return fmt.Errorf("unexpected consumer %q", consumer)
			}
			// TODO: publish VideoChunkUploaded (media_time 42 * ~2 s); delivery is only
			// observable by the resulting ChunkTranscriptIndexed.
			return pending(fCold, "publish %s for %s", event, consumer)
		})
	r.Step(`^the AI Worker extracts the audio track and transcribes it with "([^"]+)"$`,
		func(ctx context.Context, sttModel string) error {
			// TODO: wait (ColdPathTimeout) for ChunkTranscriptIndexed; stt_model == sttModel.
			return pending(fCold, "ChunkTranscriptIndexed.stt_model == %s", sttModel)
		})
	r.Step(`^the detected language "([^"]+)" is recorded in the transcript$`,
		func(ctx context.Context, lang string) error {
			// TODO: s3.Get(transcript) -> Codec.DecodeTranscriptJSON; language == lang; text
			// non-empty and mentions the topic (case-insensitive, fuzzy).
			return pending(fCold, "transcript language == %s", lang)
		})
	r.Step(`^writes the transcript to "([^"]+)"$`,
		func(ctx context.Context, raw string) error {
			o, err := objectArg(ctx, raw)
			if err != nil {
				return err
			}
			// TODO: s3.AssertExists(o, application/json); ChunkTranscriptIndexed.transcript key == o.
			return pending(fCold, "transcript %s exists", o)
		})
	r.Step(`^embeds the text with "([^"]+)" using the "([^"]+)" prefix$`,
		func(ctx context.Context, model, prefix string) error {
			if model != contracts.EmbeddingModel {
				return fmt.Errorf("embedding model %q, contract says %q", model, contracts.EmbeddingModel)
			}
			// TODO: the prefix is internal. Black-box check: embed "passage: "+text via
			// svc.embed.v1.embed with PURPOSE_PASSAGE and compare cosine >= 0.999 with the
			// stored Qdrant vector (needs with_vector=true).
			return pending(fCold, "stored vector == embed(%q + transcript) with %s", prefix, model)
		})
	r.Step(`^upserts one Qdrant point with id UUIDv5\("([^"]+)"\) into alias "([^"]+)" \(collection "([^"]+)"\)$`,
		func(ctx context.Context, name, alias, collection string) error {
			id, err := pointArg(ctx, name)
			if err != nil {
				return err
			}
			// TODO: qdrant.AliasTarget(alias)==collection; GetPoint(alias, id) found;
			// CountByVideo(V)==1. Point-ID namespace is a TODO constant (see contracts).
			return pending(fCold, "point %s in %s -> %s", id, alias, collection)
		})
	r.Step(`^the point payload has thumbnail_key "([^"]+)", language "([^"]+)" and the chunk's start_ms/end_ms$`,
		func(ctx context.Context, rawKey, lang string) error {
			key, err := w(ctx).Expand(rawKey)
			if err != nil {
				return err
			}
			if err := contracts.ValidateKey(key); err != nil {
				return err
			}
			// TODO: payload thumbnail_key == key, language == lang, start_ms/end_ms equal the
			// VideoChunkUploaded media_time; plus video_id, chunk_seq, transcript_key, models.
			return pending(fCold, "payload thumbnail_key=%s language=%s with media time", key, lang)
		})
	r.Step(`^"ChunkTranscriptIndexed" with outcome (\w+), embedding_model "([^"]+)" and qdrant_collection "([^"]+)" is published on "([^"]+)"$`,
		func(ctx context.Context, outcome, model, collection, rawSubject string) error {
			subj, err := subjectArg(ctx, rawSubject)
			if err != nil {
				return err
			}
			// TODO: WaitFor(OnSubject(subj) && WithMsgID(ai-indexed:V:000042)); decode and check
			// outcome, embedding_model, qdrant_collection (physical name), point_id.
			return pending(fCold, "ChunkTranscriptIndexed %s %s %s on %s", outcome, model, collection, subj)
		})

	r.Step(`^a chunk with (silent audio|no audio track) is indexed$`,
		func(ctx context.Context, kind string) error {
			fixture := map[string]string{"silent audio": "chunk_silent_audio.ts", "no audio track": "chunk_no_audio.ts"}[kind]
			// TODO: upload FixturesDir/fixture as a new chunk of V and publish its event.
			return pending(fCold, "index %s fixture %s", kind, fixture)
		})
	r.Step(`^"ChunkTranscriptIndexed" is published with outcome NO_SPEECH and no Qdrant point is created$`,
		func(ctx context.Context) error {
			// TODO: wait for event; outcome NO_SPEECH; transcript written; GetPoint(id) not found.
			return pending(fCold, "NO_SPEECH and no Qdrant point")
		})
	r.Step(`^"ChunkTranscriptIndexed" is published with outcome NO_AUDIO and no transcript is written$`,
		func(ctx context.Context) error {
			// TODO: wait for event; outcome NO_AUDIO; transcript unset; AssertAbsent(transcript);
			// no point either (B3: points only for INDEXED).
			return pending(fCold, "NO_AUDIO and no transcript")
		})

	// ---- @load ----
	r.Step(`^consumer "([^"]+)" has no pending messages$`,
		func(ctx context.Context, consumer string) error {
			// TODO: js.Consumer(VIDEO_EVENTS, consumer).Info: NumPending == 0 && NumAckPending == 0.
			return pending(fCold, "consumer %s has no backlog", consumer)
		})
	r.Step(`^(\d+) speech chunks are published one at a time$`,
		func(ctx context.Context, n int) error {
			// TODO: for i<n: publish, wait for ChunkTranscriptIndexed, record event timestamp
			// minus uploaded stream timestamp in w.Latency("cold-path").
			return pending(fCold, "publish %d speech chunks sequentially", n)
		})
	r.Step(`^p95 latency from publish to "ChunkTranscriptIndexed" is at most (\d+) seconds$`,
		func(ctx context.Context, seconds int) error {
			// TODO: w.Latency("cold-path").AssertAtMost(95, seconds*time.Second, 50).
			return pending(fCold, "p95 publish-to-indexed <= %d s", seconds)
		})
}
