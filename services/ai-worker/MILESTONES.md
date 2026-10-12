# AI Worker and Embedder milestones (AI0–AI4)

Python. One image, two entrypoints:
- **AI Worker** (consumer `ai-indexer`): transcribes each chunk with faster-whisper, stores
  the transcript, embeds it and upserts one Qdrant point per chunk.
- **Embedder**: answers `svc.embed.v1.embed` requests (used by the Serving API for queries)
  with the same embedding module.

Source of truth: spec §2.3, A17, Feature 3 / 4 / 5.2b; [`docs/architecture-review.md`](../../docs/architecture-review.md)
decisions 8, 9, 11 and B1/B3; required tests in [`docs/test-plan.md`](../../docs/test-plan.md)
§1.2 "AI Worker / Embedder". Uses `libs/python/af_contracts` and `gen/python` (C4).

Stack: `nats-py`, `faster-whisper`, `qdrant-client`, the embedding library chosen in AI0.
Tests use `pytest` with fakes for NATS, S3 and Qdrant; the speech fixtures are
`tests/fixtures/media/speech/chunks/*.ts` and `chunk_speech_en.ts`.

**Known issue from the fixture work (2026-10-12):** faster-whisper 1.1.1 and 1.2.1 call
`av.open(..., metadata_errors=...)`, which current PyAV removed. Either pin a PyAV release that
still has it (needs a wheel for the target platform) or decode audio with the ffmpeg CLI and
pass a NumPy array, as `tests/tools/transcribe_chunks.py` does. AI0 decides.

---

## AI0. Spikes: embedding library and Whisper baking

**Status:** planned · **Depends on:** — · **Parallel with:** C1–C3, H0

**Scope**
- **Embedding library (decision D3).** Compare fastembed custom ONNX, onnxruntime +
  `tokenizers`, and sentence-transformers for `intfloat/multilingual-e5-small` (mean pooling,
  L2 normalise, `query: ` / `passage: ` prefixes). Pick the first that matches the reference
  sentence-transformers vectors at cosine ≥ 0.999 on a golden set (English plus a few
  Japanese and Chinese sentences, for later). Record image size and CPU latency per text.
- Commit the golden set: `services/ai-worker/testdata/embed_golden.json` (texts, purpose,
  reference vectors).
- **Whisper.** Bake the `WHISPER_MODELS` weights (default `small`) into a test image; confirm
  startup fails when `WHISPER_MODEL` is not baked in; settle the PyAV issue above.
- Confirm the speech fixtures transcribe as expected inside the worker's own image (should
  match the `tools-image` result: 8/8 "Data Pipeline Architecture", en).

**Verify**
```sh
python -I services/ai-worker/spikes/embed_compare.py     # prints cosine per text and library; min must be ≥ 0.999
python -I services/ai-worker/spikes/whisper_check.py tests/fixtures/media/speech/chunks 0 7
```

**Report includes:** the comparison table (library, min cosine, image size, ms per text) and
the decision for D3, for your approval before AI1.

---

## AI1. Embedding module and Embedder service

**Status:** planned · **Depends on:** AI0 (D3 approved), C4 · **Unblocks:** API3

**Scope**
- Shared module `af_embed`: adds `query: ` / `passage: ` exactly once based on `Purpose`;
  rejects `PURPOSE_UNSPECIFIED`; returns `EmbedResponse{model, dim=384, vectors}`.
- Embedder entrypoint: NATS request-reply on `svc.embed.v1.embed`, batch limit, error replies.
- Startup refusal if alias `transcripts` does not resolve to `transcripts_me5s384_v1`.

**Verify**
```sh
(cd services/ai-worker && python -m pytest -q tests/test_embed.py tests/test_embedder_service.py)
```
The golden-vector test (cosine ≥ 0.999) runs here on every change.

**Report includes:** min cosine on the golden set; request/reply round trip against an
in-process `nats-server`.

---

## AI2. AI Worker pipeline

**Status:** planned · **Depends on:** AI1

**Scope**
- Per `VideoChunkUploaded`: download + sha256 check, decode audio, transcribe, write the
  transcript as proto3 JSON to `af-media/videos/{vid}/transcripts/{seq6}.json`, embed
  (`PURPOSE_PASSAGE`), upsert point UUIDv5(`{video_id}:{seq6}`) with `wait=true` and the B3
  payload (`thumbnail_key` from the fixed pattern, without waiting for the Stream Worker),
  publish `ChunkTranscriptIndexed`, then ack.
- Outcomes: speech → `INDEXED`; silence → `NO_SPEECH` (transcript, no point); no audio stream
  → `NO_AUDIO` (no transcript, no point).
- Text sanitising (non-UTF-8, control characters) before the JSON is written.
- `stt_model` = actual model, e.g. `faster-whisper/small/int8`; language from Whisper per chunk.

**Verify**
```sh
(cd services/ai-worker && python -m pytest -q tests/test_pipeline.py)
```
With fixtures: `chunk_speech_en.ts` → INDEXED, en, text contains "data pipeline";
`chunk_silent_audio.ts` → NO_SPEECH; `chunk_no_audio.ts` → NO_AUDIO. Point IDs checked
against `contracts/testdata/keys.json`.

**Report includes:** the outcome per fixture with transcript text and processing time.

---

## AI3. Failure handling and fault hooks

**Status:** planned · **Depends on:** AI2 · **Unblocks:** ACC3

**Scope**
- The same DLQ decision table as SW1, with NAK delays 2 s / 10 s / 30 s / 60 s; Qdrant
  unavailable is transient.
- `AI_PIPELINE_TIMEOUT` (45 s default) and `AckProgress` every 15 s.
- Fault hooks (spec 2.5) with the same semantics as the Stream Worker, consumer `ai-indexer`.
- Crash-before-ack redelivery leaves one transcript, one point, one event (Feature 5.2b).

**Verify**
```sh
(cd services/ai-worker && python -m pytest -q tests/test_dlq.py tests/test_faults.py tests/test_idempotency.py)
```

**Report includes:** the decision table as tested; the redelivery result.

---

## AI4. Image, startup refusals and latency

**Status:** planned · **Depends on:** AI3

**Scope**
- Image `affogato/ai-worker:dev` with weights baked in (`WHISPER_MODELS` build arg) and the
  embedding model; entrypoints `worker` and `embedder`; non-root.
- Startup refusals: model not baked in; alias mismatch.
- Single-chunk latency on this machine's CPU, compared with the 5 s p95 cold-path target
  before the `@load` run (ACC4).

**Verify**
```sh
(cd services/ai-worker && python -m pytest -q)
docker build -t affogato/ai-worker:dev services/ai-worker
docker run --rm -e WHISPER_MODEL=large-v3 affogato/ai-worker:dev worker   # must exit non-zero: not baked in
```

**Report includes:** image size; latency p50/p95 over the 18 speech chunks.

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
