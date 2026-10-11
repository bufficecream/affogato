# FramePulse: Architecture Review & Data Contracts (Draft)

Produced by the architect agent on 2026-10-10; **updated 2026-10-11** with the owner's decisions (see "Resolved decisions (2026-10-11)" at the end). Status: **contracts agreed — ready for implementation.** QA gap decisions G1–G6 (`docs/test-plan.md`) applied 2026-10-11; see resolved decisions 14–19. Line references (Lxx) in Part A point into the original spec, `docs/framepulse_spec.v1.md`; the current spec is `framepulse_spec.md` (v3).

---

# Part A: Spec and architecture review

## 1. Summary
The basic design is sound: events carry a pointer to the object in MinIO (never video bytes), and the hot path (HLS) and cold path (AI) are two independent consumers of the same events. The spec, however, is a set of BDD acceptance examples rather than a design. It contradicts itself on names, formats and languages, and leaves out the hard parts:
- who owns the playlist when several workers write to it
- how a 2-second chunk turns into a 15-second search hit
- how the Go Serving API embeds a search query with no model
- what "within 500 ms" actually measures

All of these shape the shared contracts, so each needs a decision before any service is written.

## 2. Findings

| # | Area | Issue | Impact | Evidence |
|---|---|---|---|---|
| F1 | Contracts | Topic is `video.chunks` in two places and `video.events` in the diagram | High | L8, L23, L53 |
| F2 | Languages | "Go - Axum/Gin": Axum is Rust; the diagram says the Serving API is Go | Med | L13, L36 |
| F3 | Ingest format | Glossary says Producer cuts `.ts`, BDD uploads `sample_01.mp4`; `chunk_01.ts` has no zero-padding and leaves 0- vs 1-based numbering open | High | L7, L52, L55 |
| F4 | Hot path | "HLS Transcoding" of chunks already in `.ts`: transcode vs remux undefined, renditions undefined; CPU 1080p transcode in <500 ms is unrealistic | High | L9, L28, L54 |
| F5 | Hot path | Per-chunk `playlist.m3u8` generation with multiple replicas and out-of-order chunks is a read-modify-write race; live vs VOD and `#EXT-X-ENDLIST` undefined; no end-of-video signal | High | L29, L55 |
| F6 | Latency | "Consume within 500 ms" ambiguous (delivery vs playable segment); 5 s cold-path figure has no start point and ignores backlog | Med | L54, L78 |
| F7 | Fan-out | "Consumer group" isn't a JetStream concept; fan-out needs two durable consumers on a Limits/Interest stream (WorkQueue would reject the second consumer) | High | L25, L75 |
| F8 | Failure handling | DLQ only specified for hot path; no cold-path failures, redrive, or poison-pill crash loops. In-process libav decoding of corrupt media can segfault the pod | High | L58–63 |
| F9 | Search | Query must be embedded with the indexing model; Go Serving API has no model — missing component | High | L91, L36 |
| F10 | Search | Expected result `00:01:25 - 00:01:40` is 15 s but chunks are 2 s; Whisper on isolated 2 s clips is weak | High | L7, L94 |
| F11 | Thumbnails | Search needs a thumbnail per hit; diagram omits thumbnails; AI worker has no way to know the key | Med | L9, L28, L96 |
| F12 | Ownership | AI worker "Python/Go" undecided; STT and embeddings belong in Python | Med | L10, L27 |
| F13 | Testability | "Given … in Kubernetes" ties acceptance tests to a cluster; no versioned API paths. **Resolved 2026-10-11:** testing is Kubernetes-only — acceptance tests on a local `kind` cluster with the production Helm chart, unit tests without infrastructure; API paths versioned under `/api/v1` | Low | L51, L74, L91 |
| F14 | Security/tenancy | No auth, no tenancy, no per-service NATS/MinIO permissions | Low (MVP) | — |

## 3. Decisions before coding (with recommendations)

### P0 — block the contracts
1. **Topic naming (F1).** Stream `VIDEO_EVENTS`; subjects `video.chunks.<event>.<video_id>`: `video.chunks.uploaded.{id}`, `video.chunks.hls.ready.{id}`, `video.chunks.ai.indexed.{id}`, `video.lifecycle.completed.{id}`. Dead letters on a separate `VIDEO_DLQ` stream, subject `dlq.{consumer}.{video_id}`.
2. **Raw chunk format (F3).** MPEG-TS, H.264 + AAC, starts on IDR, ~2 s, continuous PTS. The Producer rejects any other source codec up front, because the hot path only remuxes (spec A12). Producer accepts `.mp4` and cuts with `ffmpeg -c copy -f segment -segment_format mpegts -segment_time 2`. `chunk_seq` 0-based, contiguous. Durations are measured, never assumed to be 2000 ms. A video is at most 4 h / 8,000 chunks (spec A13, resolved decision 14): the Producer rejects longer files up front and rolls camera sessions over to a new `video_id`.
3. **Hot path scope (F4, F6).** MVP remuxes (`-c copy`) to one rendition `src`, validates with ffprobe, grabs one thumbnail. 500 ms SLO = p99 publish-to-delivery; add segment-ready SLO p95 1.5 s (publish until the segment is READY in `HLS_INDEX`). Budget ≈ 400–500 ms.
4. **Playlist ownership (F5).** Segment state in NATS KV `HLS_INDEX`, key `{video_id}.{rendition}`, CAS updates. MVP is VOD-only (decided 2026-10-11): the Serving API does not render playlists. The Stream Worker writes static `playlist.m3u8` (`#EXT-X-PLAYLIST-TYPE:VOD`, `#EXT-X-ENDLIST`) and `master.m3u8` once at finalization (when the index holds `expected_chunk_count` from `VideoIngestCompleted` and every chunk is READY/FAILED). It renders them deterministically from the index sorted by `chunk_seq` (out-of-order processing is expected), PUTs both, then CAS-sets `finalized=true`, so concurrent or repeated finalization writes identical bytes and the API never sees a finalized video without playlists. FAILED chunks are omitted and the next READY segment is preceded by `#EXT-X-DISCONTINUITY`. Playlists are kept after finalization. Live playback (API-rendered `EVENT` playlist from `HLS_INDEX`) is spec Future Work item 1; the KV design already supports it. The Stream Worker writes the FAILED status (CAS on `HLS_INDEX`) in the same step where it dead-letters the chunk.
5. **Fan-out (F7).** One Limits-retention stream, two durable pull consumers `hls-stream-worker` and `ai-indexer`; replicas share their consumer. Limits (not Interest) to allow replay/reindex within the 7 d `max_age`; older reprocessing reads stored artifacts from `fp-media` (transcripts for re-embedding, HLS segments for re-transcription).
6. **Failures and DLQ (F8).**
   - Classify errors permanent vs transient.
   - Permanent → explicit republish to DLQ, then `Term()`.
   - Transient → `Nak(delay)` on attempts 1–4 with worker-chosen exponential delays.
   - Worker cap: at most 5 processing attempts. A transient error on attempt 5 dead-letters (`RETRIES_EXHAUSTED`, `delivery_count` 5) and `Term()`s immediately; a message received with `num_delivered ≥ 6` (only after a crash or failed DLQ publish during attempt 5) is dead-lettered on receipt without processing (catches crash loops). See B1 "DLQ mechanism".
   - Server `max_deliver = 10` as a ceiling only; its MAX_DELIVERIES advisory feeds alerting.
   - ffmpeg/ffprobe always as subprocesses with hard timeout and `kill_on_drop`.
7. **Idempotency.** Deterministic `Nats-Msg-Id`, object keys, Qdrant point IDs (UUIDv5), set-semantic KV CAS. Order: outputs → KV → publish result (await PubAck) → ack input.
8. **Query embedding (F9).** New `embedder` deployment (AI-worker image, different entrypoint) on NATS request-reply `svc.embed.v1.embed`; Serving API calls it.
9. **Search granularity (F10).** MVP: one Qdrant point per chunk. Search fetches top 50, groups by video, merges consecutive `chunk_seq` (gap ≤ 1) into ranges, max score, joined snippet, best chunk's thumbnail. Future Work (spec §6): 10 s sliding windows behind the same alias; previous-chunk audio context for Whisper.
10. **Thumbnails (F11).** Stream Worker writes `videos/{id}/thumbs/{seq:06}.jpg`; AI worker stores that deterministic key in Qdrant without waiting; API falls back to a placeholder.
11. **Languages (F2, F12).** Producer + Serving API in Go (Gin; drop Axum). Stream Worker in Rust (`async-nats`, `aws-sdk-s3` path-style, `prost`, `tokio`, ffmpeg CLI). AI worker + embedder in Python (`nats-py`, `faster-whisper`, `qdrant-client`).
    - **Whisper:** multilingual models only, `WHISPER_MODEL` = tiny|base|small|medium|large-v3, default `small` int8 on CPU. Weights for build arg `WHISPER_MODELS` (default `small`) are baked into the image, and startup fails if the selected model is not baked in. `stt_model` records the actual model.
    - **Embedding:** `intfloat/multilingual-e5-small` (384-d, cosine, mean pooling + L2 normalize). The shared Python embedding module adds the `query: ` / `passage: ` prefix based on `Purpose`. The model is pinned, not a runtime setting.
    - **Embedding library — to be verified by the developer:** fastembed's built-in list does not include this model (only `multilingual-e5-large`, checked 2026-10-11). Preferred order: fastembed custom ONNX model, then onnxruntime + `tokenizers`, then sentence-transformers (pulls PyTorch into the image). The chosen library must match reference sentence-transformers vectors (cosine ≥ 0.999) on a committed golden set.

### P1
12. **IDs.** `video_id` = UUIDv7, lowercase hyphenated.
13. **Infra owns stream/consumer/KV config** via NACK CRDs rendered by the Helm chart from `contracts/nats/*.json` (identical in `kind` and production); services only verify existence.
14. **Serving exposure.** VOD only. Playlist endpoints return 404 for an unknown video, 409 + JSON status while processing, and after finalization proxy the static playlist content from MinIO. They proxy rather than redirect so relative URIs resolve to API routes. Segments/thumbnails are 302 to presigned GET (TTL 10 min) signed against `MINIO_PUBLIC_ENDPOINT`.
15. **Autoscaling.** KEDA on consumer `num_pending`; backpressure via fetch size + `max_ack_pending`.
16. **Test env (decided 2026-10-11).** Kubernetes-only; no docker compose, no testcontainers.
    - Unit tests per service run with no infrastructure (in-memory fakes for NATS, S3 and Qdrant; ffmpeg tests use committed fixture media).
    - Acceptance/BDD tests (godog) run against a local `kind` cluster, locally and in CI. The production Helm chart `deploy/helm/framepulse` is installed with `values-kind.yaml`.
    - Fault injection scales workloads with `kubectl`; crashes and missing acks use the test-only fault hooks (B1 "Test-only fault hooks"), enabled only by `values-kind.yaml`.
    - "Then" steps wait on result events via ordered consumers filtered by `video_id` (four filter subjects, B1 "Per-video filters").
    - `values-kind.yaml` sets `FINALIZE_IDLE_TIMEOUT=10s` and `FINALIZE_SWEEP_INTERVAL=2s` (production defaults `5m` / `30s`).

### P2
Moved to spec §6 "Future Work": live playback, video deletion, multi-bitrate ladder, windowed embeddings, Japanese/Chinese tuning, auth/tenancy (NATS accounts with least-privilege permissions, `tenant_id`), GPU node pool, manual finalize command (idle-timeout finalization is in the MVP; see resolved decision 11), index sharding for videos longer than 4 h (resolved decision 14).

---

# Part B: Data contracts

## B1. NATS JetStream

### Streams

| Setting | `VIDEO_EVENTS` | `VIDEO_DLQ` | `FP_ADVISORIES` (alerting only) |
|---|---|---|---|
| subjects | `video.chunks.>`, `video.lifecycle.>` | `dlq.>` | `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.VIDEO_EVENTS.>` |
| retention | limits | limits | limits |
| storage | file (S2) | file | file |
| replicas | 3 (dev: 1) | 3 (dev: 1) | 3 (dev: 1) |
| max_age | 7d | 7d | 7d |
| max_bytes | 10 GiB | 1 GiB | 100 MiB |
| max_msg_size | 64 KiB | 256 KiB | default |
| discard | old | **new** (failed DLQ publish → worker NAKs) | old |
| duplicate_window | 10m | 10m | 2m |
| deny_delete / deny_purge | true / false | true / false | — |
| allow_direct | true | true | — |

Subjects (last token always `video_id`):
- `video.chunks.uploaded.{video_id}` — Producer
- `video.chunks.hls.ready.{video_id}` — Stream Worker
- `video.chunks.ai.indexed.{video_id}` — AI worker
- `video.lifecycle.completed.{video_id}` — Producer
- `dlq.{consumer}.{video_id}` (`unknown` if payload undecodable)
- `svc.embed.v1.embed` — core NATS request-reply, not stored

**Per-video filters.** NATS `*` matches exactly one token and event subjects have 4 or 5 tokens, so `video.chunks.*.{video_id}` alone matches only `uploaded`. A subscription for every message of one video needs:

| Stream | Filter subject | Matches |
|---|---|---|
| `VIDEO_EVENTS` | `video.chunks.*.{video_id}` | `uploaded` |
| `VIDEO_EVENTS` | `video.chunks.*.*.{video_id}` | `hls.ready`, `ai.indexed` |
| `VIDEO_EVENTS` | `video.lifecycle.*.{video_id}` | `completed` (and future lifecycle events) |
| `VIDEO_DLQ` | `dlq.*.{video_id}` | dead letters from every consumer (not `dlq.{consumer}.unknown`) |

New event subjects must keep `video_id` as the last token and fit one of these shapes. `contracts/testdata/keys.json` lists the filters per video; the contracts libraries expose a builder for them.

Schema version lives in the header and proto package, not the subject.

KV bucket `HLS_INDEX`: history 1, R3, file, max_value_size 1 MiB; key `{video_id}.{rendition}`; value `framepulse.state.v1.HlsIndex`. **Size budget:** a `Segment` entry encodes to ≤ 19 B (`chunk_seq` ≤ 3-byte varint, `media_time` ms as ≤ 4-byte varints, status; no key string), so the 8,000-chunk cap (spec A13) bounds the value at ≈ 152 KB + ~60 B of header fields, 6.9× below `max_value_size` and the NATS `max_payload` (1 MiB). With the former `Segment.key` (~82 B per entry) 8,000 chunks would be ≈ 656 KB (1.6×, under the required 2× margin), and 1 MiB would be hit at ~12,300 chunks (~6.8 h). Each CAS rewrites the whole value (≈ 640 MB written over the life of an 8,000-chunk video, ×3 replicas); acceptable for the MVP, and index sharding is spec Future Work 9. It tracks per-segment READY/FAILED status, measured durations and the `finalized` flag. In the VOD-only MVP the Serving API reads it only to decide playlist responses (404 / 409 / 200).

### Consumers (durable, pull)

| | `hls-stream-worker` | `ai-indexer` |
|---|---|---|
| filter_subjects | `video.chunks.uploaded.*`, `video.lifecycle.completed.*` | `video.chunks.uploaded.*` |
| deliver_policy | all | all |
| ack_policy | explicit | explicit |
| ack_wait | 15s (ffmpeg timeout 10s) | 60s (pipeline timeout 45s for the default `small` model, configurable via `AI_PIPELINE_TIMEOUT`; `AckProgress` every 15s keeps longer runs alive) |
| max_deliver | 10 (server ceiling; worker cap: 5 processing attempts) | 10 (server ceiling; worker cap: 5 processing attempts) |
| backoff | worker NAK delays after attempts 1–4: 1s, 2s, 5s, 15s | worker NAK delays after attempts 1–4: 2s, 10s, 30s, 60s |
| max_ack_pending | 512 | 32 |
| client fetch | `consume()` max_messages = cores, heartbeat 5s | max_messages = 1 per process |

**DLQ mechanism — worker republishes explicitly:**
Attempt n is the delivery with `num_delivered` = n; a message gets at most 5 processing attempts.
1. On receipt, `num_delivered ≥ 6` → publish `DeadLetter` (`ERROR_CLASS_RETRIES_EXHAUSTED`, `delivery_count` = `num_delivered`) without processing, then `Term()`. Only reachable after a crash (no ack/nak) or a failed DLQ publish during attempt 5.
2. Test-only fault hook, if enabled and requested for this consumer (see "Test-only fault hooks").
3. Permanent error on any attempt → `DeadLetter` (its error class, `delivery_count` = n), `Term()`.
4. Transient error on attempts 1–4 → `Nak(delay[n])`.
5. Transient error on attempt 5 → `DeadLetter` (`ERROR_CLASS_RETRIES_EXHAUSTED`, `delivery_count` 5), `Term()` immediately; no 5th `Nak`.
6. DLQ publish fails (steps 1, 3, 5) → `Nak(delay)`, never `Term()`; the redelivery then hits step 1. Server `max_deliver = 10` stays a ceiling whose MAX_DELIVERIES advisory only feeds alerting.

**Redrive** (`fpctl dlq redrive --consumer X --since T`): republish `original_payload` to `original_subject` with original headers, `Nats-Msg-Id = {orig}:redrive:{n}`. Redrive only works while the raw chunk exists, so the `fp-raw` expiry (10 d) must stay longer than the `VIDEO_DLQ` `max_age` (7 d). Redrive drops the `Fp-Test-Fault` header from the republished headers.

### Test-only fault hooks (spec 2.5)

Purpose: make "crash loop" (spec Feature 4) and "processed but not acked" (Feature 5) deterministic in acceptance tests. kind and production run the same images; the hooks are gated at runtime, not compiled out, so the tested binary is the shipped binary.

**Enablement**
- Env `FP_TEST_FAULTS`: unset, empty or `0` → disabled (header ignored, not parsed); `1` → enabled; any other value → startup fails with a clear log line.
- An enabled worker logs a startup warning and exports gauge `fp_test_faults_enabled` = 1 (alert if it is ever 1 outside kind).
- Helm value `testFaults.enabled` (default `false`) renders `FP_TEST_FAULTS=1` into the Stream Worker and AI Worker deployments. Only `values-kind.yaml` sets it. CI step 7 fails if `helm template -f values-prod.yaml` output contains `FP_TEST_FAULTS`.
- Producer, Serving API and Embedder have no hooks.
- The MVP has no NATS auth (spec A9), so anyone who can publish can set the header; the env gate is the only control and must stay enforced in CI.

**Header grammar** (one header value per fault; several values allowed):
```
Fp-Test-Fault: <fault>; consumer=<durable>[; deliveries=<n>]
<fault>   = exit-on-receive | crash-before-ack | fail-transient
<durable> = hls-stream-worker | ai-indexer
<n>       = positive integer; the fault fires only while num_delivered <= n
```
- Scoped per message and per consumer: a worker applies only values whose `consumer` is its own durable name; the other path processes the message normally.
- Honored on any message delivered to that consumer (`VideoChunkUploaded`, `VideoIngestCompleted`).
- When enabled, a malformed value (unknown fault or parameter, missing `consumer`, bad `n`) → `DeadLetter` `ERROR_CLASS_SCHEMA`, `error_code` `TEST_FAULT_INVALID`, `Term()`; never silently ignored.
- Order in the worker: delivery-cap check (DLQ step 1) → fault hook → processing.
- Workers never copy `Fp-Test-Fault` onto result events; `DeadLetter.original_headers` keeps it verbatim, and redrive strips it.

| Fault | Fires | Effect | Default `n` | Stream Worker | AI Worker | Used by |
|---|---|---|---|---|---|---|
| `exit-on-receive` | after the cap check, before any side effect | process exits with code 70, no ack/nak; redelivered after `ack_wait` | unlimited | yes | yes | Feature 4 crash loop: deliveries 1–5 crash, delivery 6 is dead-lettered on receipt with `delivery_count` 6 |
| `crash-before-ack` | after outputs, KV update and the result event's PubAck, in place of the ack | process exits with code 70 | 1 | yes | yes | Feature 5 "processed but not acked": delivery 2 is processed and acked normally |
| `fail-transient` | before any side effect | handled as a transient error (DLQ steps 4–5) | unlimited | yes | yes | DLQ-cap checks (`delivery_count` 5) without scaling dependencies; recovery tests with `deliveries=k` |

Timing in kind: each crashed delivery waits one `ack_wait` (15 s hot path, 60 s AI) plus the container restart, so the AI crash-loop scenario takes about 5–6 min; `crash-before-ack` costs one `ack_wait`.

### Headers

| Header | Value |
|---|---|
| `Nats-Msg-Id` | `uploaded:{video_id}:{seq:06}`, `hls-ready:{video_id}:{rendition}:{seq:06}`, `ai-indexed:{video_id}:{seq:06}`, `completed:{video_id}`, `dlq:{consumer}:{original_stream_seq}` |
| `Content-Type` | `application/protobuf` |
| `Fp-Schema` | fully qualified message name, e.g. `framepulse.events.v1.VideoChunkUploaded`; unknown → DLQ as `ERROR_CLASS_SCHEMA` |
| `Fp-Producer` | e.g. `stream-worker/1.4.2` |
| `traceparent` / `tracestate` | W3C, lowercase |
| `Fp-Test-Fault` | Test only; honored only when the worker runs with `FP_TEST_FAULTS=1` (kind). `<fault>; consumer=<durable>[; deliveries=<n>]`, see "Test-only fault hooks". Never copied onto result events; stripped by redrive |

Latency metric: `fp_consume_latency_seconds = now − msg.metadata.timestamp`.

### Proto files

`proto/framepulse/common/v1/common.proto`
```proto
syntax = "proto3";

package framepulse.common.v1;

import "google/protobuf/timestamp.proto";

option go_package = "github.com/framepulse/framepulse/gen/go/framepulse/common/v1;commonv1";

// Design note: video_id is a plain string (no wrapper message) to avoid
// Option<VideoId> / HasField friction in prost and Python. Format rule:
// lowercase canonical UUIDv7, 36 chars, e.g. "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f".

// Identifies one chunk of one video.
message ChunkRef {
  string video_id = 1;
  // 0-based, contiguous, assigned by the producer, unique within video_id.
  uint32 chunk_seq = 2;
  // Field 3 is held for tenant_id (spec A9 and Future Work). Do not use it for anything else.
}

// Location-independent pointer to an S3/MinIO object. Never contains an
// endpoint or URL; readers resolve the endpoint from their own config.
message ObjectRef {
  string bucket = 1;
  string key = 2;
  int64 size_bytes = 3;
  string content_type = 4;
  // Lowercase hex SHA-256 of the full body. Empty if not computed.
  string sha256_hex = 5;
  // ETag returned by PUT. Informational only (multipart ETags are not MD5).
  string etag = 6;
}

// Half-open media-time interval [start_ms, end_ms) relative to the first
// frame of chunk 0 of the video. Not wall-clock time.
message TimeRange {
  int64 start_ms = 1;
  int64 end_ms = 2;
}

// Result of ffprobe on a chunk.
message MediaInfo {
  string container = 1;          // "mpegts"
  string video_codec = 2;        // "h264"
  string audio_codec = 3;        // "aac"; empty if no audio
  uint32 width = 4;
  uint32 height = 5;
  uint32 frame_rate_num = 6;
  uint32 frame_rate_den = 7;
  bool has_audio = 8;
  uint32 audio_sample_rate_hz = 9;
  uint32 audio_channels = 10;
  uint64 bitrate_bps = 11;
}

// Envelope metadata present as field 1 of every event.
message EventMeta {
  // UUIDv7, generated once per logical event and reused on publish retries.
  string event_id = 1;
  google.protobuf.Timestamp occurred_at = 2;
  string producer = 3;           // service name, e.g. "stream-worker"
  string producer_version = 4;   // semver or git sha
  // event_id of the event that caused this one; empty for root events.
  string causation_id = 5;
}
```

`proto/framepulse/events/v1/chunk_events.proto`
```proto
syntax = "proto3";

package framepulse.events.v1;

import "framepulse/common/v1/common.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/framepulse/framepulse/gen/go/framepulse/events/v1;eventsv1";

// Subject:     video.chunks.uploaded.{video_id}
// Nats-Msg-Id: uploaded:{video_id}:{chunk_seq:06}
// Publisher:   producer. Published only after the raw object PUT succeeded.
message VideoChunkUploaded {
  framepulse.common.v1.EventMeta meta = 1;
  framepulse.common.v1.ChunkRef chunk = 2;
  // fp-raw/videos/{video_id}/chunks/{seq:06}.ts; sha256_hex is REQUIRED.
  framepulse.common.v1.ObjectRef raw_object = 3;
  // Measured from source PTS (keyframe-aligned cuts are not exactly 2000 ms).
  framepulse.common.v1.TimeRange media_time = 4;
  framepulse.common.v1.MediaInfo media = 5;
  // Camera / upload source identifier, free-form, observability only.
  string source_id = 6;
  // Wall-clock capture time of first frame if known (cameras); unset for files.
  google.protobuf.Timestamp captured_at = 7;

  // End-of-video is signalled by VideoIngestCompleted, never per chunk.
  reserved "is_last";
}

// Subject:     video.chunks.hls.ready.{video_id}
// Nats-Msg-Id: hls-ready:{video_id}:{rendition}:{chunk_seq:06}
// Publisher:   stream-worker, after segment + thumbnail PUT and HLS_INDEX CAS.
message ChunkHlsReady {
  framepulse.common.v1.EventMeta meta = 1;
  framepulse.common.v1.ChunkRef chunk = 2;
  string rendition = 3;                          // "src" in MVP
  framepulse.common.v1.ObjectRef segment = 4;
  framepulse.common.v1.ObjectRef thumbnail = 5;
  // Actual PTS range of the output segment (authoritative for EXTINF).
  framepulse.common.v1.TimeRange media_time = 6;
  uint32 processing_ms = 7;                      // receive -> publish
}

enum IndexOutcome {
  INDEX_OUTCOME_UNSPECIFIED = 0;
  INDEX_OUTCOME_INDEXED = 1;     // point upserted
  INDEX_OUTCOME_NO_SPEECH = 2;   // audio present, empty transcript; no point
  INDEX_OUTCOME_NO_AUDIO = 3;    // no audio track; no point, no transcript
}

// Subject:     video.chunks.ai.indexed.{video_id}
// Nats-Msg-Id: ai-indexed:{video_id}:{chunk_seq:06}
// Publisher:   ai-worker, after transcript PUT and Qdrant upsert (wait=true).
message ChunkTranscriptIndexed {
  framepulse.common.v1.EventMeta meta = 1;
  framepulse.common.v1.ChunkRef chunk = 2;
  framepulse.common.v1.TimeRange media_time = 3;
  IndexOutcome outcome = 4;
  framepulse.common.v1.ObjectRef transcript = 5;  // unset if NO_AUDIO
  string language = 6;                            // detected per chunk by Whisper, BCP-47 ("en", "ja", "zh")
  string stt_model = 7;                           // actual model, e.g. "faster-whisper/small/int8" (WHISPER_MODEL)
  string embedding_model = 8;                     // "intfloat/multilingual-e5-small"
  string qdrant_collection = 9;                   // physical name, not alias
  string point_id = 10;                           // UUIDv5, see Qdrant section
  uint32 processing_ms = 11;
  uint32 transcript_chars = 12;
}
```

`proto/framepulse/events/v1/video_events.proto`
```proto
syntax = "proto3";

package framepulse.events.v1;

import "framepulse/common/v1/common.proto";

option go_package = "github.com/framepulse/framepulse/gen/go/framepulse/events/v1;eventsv1";

// Subject:     video.lifecycle.completed.{video_id}
// Nats-Msg-Id: completed:{video_id}
// Publisher:   producer, after the PubAck of the last VideoChunkUploaded.
message VideoIngestCompleted {
  framepulse.common.v1.EventMeta meta = 1;
  string video_id = 2;
  uint32 chunk_count = 3;                         // chunks 0..chunk_count-1
  framepulse.common.v1.TimeRange media_time = 4;  // whole video
}
```

`proto/framepulse/dlq/v1/dead_letter.proto`
```proto
syntax = "proto3";

package framepulse.dlq.v1;

import "framepulse/common/v1/common.proto";
import "google/protobuf/timestamp.proto";

option go_package = "github.com/framepulse/framepulse/gen/go/framepulse/dlq/v1;dlqv1";

enum ErrorClass {
  ERROR_CLASS_UNSPECIFIED = 0;
  ERROR_CLASS_INVALID_MEDIA = 1;       // corrupt / undecodable / no streams
  ERROR_CLASS_OBJECT_NOT_FOUND = 2;
  ERROR_CLASS_CHECKSUM_MISMATCH = 3;
  ERROR_CLASS_SCHEMA = 4;              // undecodable payload / unknown Fp-Schema
  ERROR_CLASS_TIMEOUT = 5;             // subprocess/pipeline timeout (permanent after cap)
  ERROR_CLASS_RETRIES_EXHAUSTED = 6;   // transient failure on attempt 5, or received with num_delivered >= 6
  ERROR_CLASS_INTERNAL = 7;
}

message Header {
  string name = 1;
  repeated string values = 2;
}

// Subject:     dlq.{consumer}.{video_id|unknown}
// Nats-Msg-Id: dlq:{consumer}:{original_stream_seq}
message DeadLetter {
  framepulse.common.v1.EventMeta meta = 1;
  // Original message, verbatim (enables redrive without re-encoding).
  string original_stream = 2;
  uint64 original_stream_seq = 3;
  string original_subject = 4;
  repeated Header original_headers = 5;
  bytes original_payload = 6;
  google.protobuf.Timestamp original_published_at = 7;
  // Failure context.
  string consumer = 8;
  uint32 delivery_count = 9;
  ErrorClass error_class = 10;
  string error_code = 11;       // stable machine code, e.g. "FFPROBE_NO_STREAMS"
  string error_message = 12;    // human-readable, truncated to 4 KiB
  bool redrivable = 13;         // false for INVALID_MEDIA / CHECKSUM_MISMATCH
  framepulse.common.v1.ChunkRef chunk = 14;  // best effort; unset if undecodable
  string worker_instance = 15;  // pod name
}
```

Supporting contracts (same conventions, fields abbreviated):
- `framepulse/state/v1/hls_index.proto`
  - `HlsIndex { string video_id=1; string rendition=2; repeated Segment segments=3 /*sorted by seq*/; uint32 expected_chunk_count=4 /*0 = unknown*/; bool finalized=5; google.protobuf.Timestamp last_activity_at=6 /*stamped on every update*/; FinalizeReason finalize_reason=7; }`
  - `enum FinalizeReason { FINALIZE_REASON_UNSPECIFIED=0; FINALIZE_REASON_COMPLETED=1; FINALIZE_REASON_IDLE_TIMEOUT=2; }`
  - `Segment { uint32 chunk_seq=1; reserved 2; reserved "key"; TimeRange media_time=3; SegmentStatus status=4; }`. The segment key is derived (`videos/{video_id}/hls/{rendition}/{seq6}.ts`, built by the contracts libraries), not stored; this keeps an entry ≤ 19 B (B1 KV size budget).
  - `enum SegmentStatus { SEGMENT_STATUS_UNSPECIFIED=0; SEGMENT_STATUS_READY=1; SEGMENT_STATUS_FAILED=2; }`
- `framepulse/artifacts/v1/transcript.proto`
  - `Transcript { ChunkRef chunk=1; TimeRange media_time=2; string language=3; string stt_model=4; string text=5; repeated TranscriptSegment segments=6; }`
  - `TranscriptSegment { TimeRange media_time=1 /*video-relative*/; string text=2; float avg_logprob=3; }`
  - Stored in MinIO as canonical proto3 JSON.
- `framepulse/embed/v1/embed.proto`
  - `EmbedRequest { repeated string texts=1; Purpose purpose=2; }`, `enum Purpose { PURPOSE_UNSPECIFIED=0; PURPOSE_QUERY=1; PURPOSE_PASSAGE=2; }`
  - `EmbedResponse { string model=1; uint32 dim=2; repeated Vector vectors=3; }`, `Vector { repeated float values=1; }`
  - Callers send unprefixed text. The shared embedding module prepends `query: ` (`PURPOSE_QUERY`) or `passage: ` (`PURPOSE_PASSAGE`) as e5 requires; `PURPOSE_UNSPECIFIED` is rejected. The Serving API checks that `EmbedResponse.model` matches the model of the collection it searches.

### Evolution rules
- `buf lint` STANDARD; `buf format` enforced.
- `buf breaking --against main` with the **FILE** rule set (generated code is an API, so renames are breaking).
- Never reuse or renumber a field; deleting means `reserved <n>; reserved "<name>";` in the same PR.
- Additive changes only; consumers accept missing fields and unknown fields/enum values. `optional` only where zero is meaningful.
- Breaking change → new `…v2` package on the same subjects with `Fp-Schema: …v2.X`; producers dual-publish until all consumers migrate.
- No `protovalidate` (uneven Rust support); validation lives in each language's contracts library.

## B2. MinIO object storage

**Buckets**
- `fp-raw` — producer input; expires after 10 days (must stay longer than `VIDEO_DLQ` retention of 7 days so redrive finds the raw chunk).
- `fp-media` — derived artifacts, including the final playlists; kept until the video is deleted (deletion is Future Work).
- Both: abort incomplete multipart uploads after 1 day; versioning off.

**Keys** (`seq6` = zero-padded 6-digit 0-based `chunk_seq`; a video has at most 8,000 chunks (4 h, spec A13), so 6 digits leave ample room and the format stays fixed; key builders reject seq > 999,999, event validation rejects `chunk_seq` ≥ 8,000):

| Artifact | Bucket / key | Example | Content-Type | Cache-Control |
|---|---|---|---|---|
| Raw chunk | `fp-raw/videos/{vid}/chunks/{seq6}.ts` | `fp-raw/videos/0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f/chunks/000042.ts` | `video/mp2t` | — |
| HLS segment | `fp-media/videos/{vid}/hls/{rendition}/{seq6}.ts` | `…/hls/src/000042.ts` | `video/mp2t` | `public, max-age=31536000, immutable` |
| Media playlist (final) | `fp-media/videos/{vid}/hls/{rendition}/playlist.m3u8` | `…/hls/src/playlist.m3u8` | `application/vnd.apple.mpegurl` | `max-age=86400` |
| Master playlist (final) | `fp-media/videos/{vid}/hls/master.m3u8` | one variant in MVP | `application/vnd.apple.mpegurl` | `max-age=86400` |
| Thumbnail | `fp-media/videos/{vid}/thumbs/{seq6}.jpg` | 320×180, first IDR of chunk | `image/jpeg` | immutable |
| Transcript | `fp-media/videos/{vid}/transcripts/{seq6}.json` | proto3 JSON `Transcript` | `application/json` | immutable |

**Object metadata** (`x-amz-meta-*`): `fp-video-id`, `fp-chunk-seq`, `fp-sha256`, `fp-producer`, `fp-source-event-id`.

**Write rules.** All keys except playlists are logically write-once (redelivery may overwrite with equivalent content). Playlists written only at finalization, before `finalized=true` is set in `HLS_INDEX`, and kept afterwards. Workers verify `sha256` after download; mismatch → DLQ `CHECKSUM_MISMATCH`.

**Serving API exposure** (MinIO stays private):
- `GET /api/v1/videos/{id}/hls/master.m3u8`, `GET /api/v1/videos/{id}/hls/{rendition}/playlist.m3u8` (VOD only, nothing rendered by the API):
  - **Unknown video or rendition** (no `HLS_INDEX` key): 404.
  - **Not finalized:** 409, `Cache-Control: no-store`, `Retry-After: 5`, body `{"video_id","status":"processing","segments_ready","segments_failed","expected_chunk_count"|null}`.
  - **Finalized:** 200 with the MinIO object's bytes proxied verbatim (`application/vnd.apple.mpegurl`, `max-age=86400`); the relative URIs resolve to the API's segment routes.
  - **Finalized but object missing:** 503 + alert (invariant violation).
- `GET /api/v1/videos/{id}/hls/{rendition}/{seq6}.ts`, `GET /api/v1/videos/{id}/thumbs/{seq6}.jpg` — 302 to presigned GET (10 min, public endpoint).
  - **Finalized with no playable segment** (idle timeout, zero READY chunks): 410 Gone, body `{"video_id","status":"failed"}`.
- `GET /api/v1/search?q=&limit=10` (alias `/api/search`). Result: `{video_id, start_ms, end_ms, timestamp:"00:01:25 - 00:01:40", snippet, thumbnail_url, playback_url, status, score}`; `thumbnail_url` is the stable API redirect URL; `status` is `processing` / `ready` / `failed` from `HLS_INDEX` (results are never hidden by status).

**Bucket notifications vs producer events.** Producer-published events only; MinIO notifications off (they lack seq, media_time, MediaInfo, sha256, and both would double-trigger). Orphaned raw objects are cleaned by lifecycle.

## B3. Qdrant
- Alias `transcripts` → physical `transcripts_me5s384_v1` (model `intfloat/multilingual-e5-small`). A new model means a new collection, re-embedding the stored transcripts from `fp-media`, then an alias swap. The AI worker and the embedder fail at startup if the alias does not resolve to their model's collection.
- Vectors: 384-dim, Cosine, default HNSW, payload on disk.
- Point ID: UUIDv5 (RFC 9562, SHA-1) with namespace **`c1907ffe-0c06-4077-8866-08fb8d658b77`** over the UTF-8 name `"{video_id}:{seq6}"` (lowercase `video_id`), written as a lowercase hyphenated UUID; upsert `wait=true`. The namespace is fixed forever: changing it changes every point ID and means a full re-index (new collection + alias swap). Golden examples: `0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f:000000` → `ce06a6cc-f9c3-5d9f-8bd0-c7212687ea4d`; `0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f:000042` → `b5e27970-263d-5af6-ba86-f5345023df69`. The constant lives in `contracts/testdata/keys.json` (`point_id_namespace`) and in each contracts library.
- Payload: `video_id` (keyword, indexed), `chunk_seq` (int, indexed), `start_ms`/`end_ms` (int, `start_ms` indexed), `text`, `language` (keyword, indexed), `thumbnail_key`, `transcript_key`, `stt_model`, `embedding_model`, `indexed_at_ms`, `source_id`.
- Points only for `INDEX_OUTCOME_INDEXED`.
- Search: embed query (`PURPOSE_QUERY`) → top 50 → merge adjacent chunks per video → top N; timestamps floor/ceil to the second, `HH:MM:SS`.

## B4. Repo layout and codegen

```
proto/                         buf module (buf.yaml v2: lint STANDARD, breaking FILE)
  framepulse/{common,events,dlq,state,artifacts,embed}/v1/*.proto
buf.gen.yaml
gen/                           committed, generated — never hand-edited
  go/        protoc-gen-go, paths=source_relative
  rust/      crate fp-proto (neoeinstein-prost + prost-crate)
  python/    dist framepulse-contracts (protocolbuffers/python + pyi)
contracts/
  nats/streams/*.json, nats/consumers/*.json, nats/kv/*.json   applied by NACK / init Job
  qdrant/transcripts_me5s384_v1.json
  testdata/*.txtpb -> *.binpb   golden fixtures via `buf convert`
  testdata/keys.json            golden vectors: inputs -> subjects, per-video filters, Msg-Ids, keys, point IDs;
                                top-level "point_id_namespace": "c1907ffe-0c06-4077-8866-08fb8d658b77"
libs/go/fpcontracts  libs/rust/fp-contracts  libs/python/fp_contracts
                               subject/key/Msg-Id/point-ID builders + validation, tested against keys.json
services/producer (Go)  services/serving-api (Go, Gin)  services/stream-worker (Rust)  services/ai-worker (Python; worker + embedder)
deploy/helm/framepulse         Helm chart: services + pinned subcharts (NATS, NACK, MinIO, Qdrant, KEDA); values-kind.yaml, values-prod.yaml
tests/acceptance               BDD runner (godog), packaged as an image, run as an in-cluster Job on kind
```

Python uses the official `protobuf` runtime (upb) + `.pyi` stubs rather than betterproto.

**CI** (`bufbuild/buf-action`):
1. `buf format --diff --exit-code`
2. `buf lint`
3. `buf breaking --against '.git#branch=main,subdir=proto'`
4. `buf generate && git diff --exit-code gen/`
5. Cross-language contract tests: every language decodes `testdata/*.binpb` and checks `keys.json`.
6. Unit tests per service, no infrastructure: `go test ./...`, `cargo test`, `pytest` (NATS/S3/Qdrant behind interfaces with in-memory fakes; ffmpeg tests use committed fixture media).
7. Chart checks: `helm lint`, `helm template … | kubeconform`; the output of `helm template -f values-prod.yaml` must not contain `FP_TEST_FAULTS`.
8. Acceptance (every PR):
   - build images
   - create a `kind` cluster
   - `kind load docker-image`
   - `helm install -f values-kind.yaml --wait`; NACK applies `contracts/nats/*.json` here, so an invalid stream/consumer/KV config fails this step
   - run the BDD Job, excluding `@load`
   - collect pod logs
9. `@load` latency scenarios: nightly or on demand on a dedicated runner with the same chart. Results are indicative, not proof of the production SLO.

---

# Resolved decisions (2026-10-11)
The owner's answers to the former open questions. The spec (§5 Assumptions & Decisions, §6 Future Work) is updated to match.
1. **Language.** English first, Japanese/Chinese later. Multilingual from day one: `intfloat/multilingual-e5-small` (384-d, cosine, `query: ` / `passage: ` prefixes chosen by `Purpose`), physical collection `transcripts_me5s384_v1`. Whisper auto-detects the language; `language` stays in events and payload for later filtering. The developer must verify the Python embedding library (decision 11).
2. **VOD only in MVP.** No API-rendered `EVENT` playlist. `HLS_INDEX` is kept for per-segment READY/FAILED, durations and finalization. Playlist endpoints return 404 for an unknown video, 409 + JSON while processing, and 200 with the proxied static playlist after finalization (not a 302, so relative URIs reach the API's segment routes). Live playback is spec Future Work item 1.
3. **Playlists kept** in `fp-media` after finalization (they are the only playlists and are tiny); space savings come from expiring raw chunks.
4. **Latency wording unchanged:** 500 ms p99 publish-to-delivery, 1.5 s p95 publish-to-segment-READY, 5 s p95 cold path without backlog. To be revisited during development.
5. **Search thumbnail URL** = stable API redirect URL (302 to presigned, 10 min).
6. **Retention.** `VIDEO_EVENTS` 7 d, `VIDEO_DLQ` 7 d, `FP_ADVISORIES` 7 d, `fp-raw` 10 d (must stay longer than the DLQ), `fp-media` and Qdrant kept until deleted. Video deletion (`VideoDeleted` cascade) is Future Work.
7. **Whisper.** `WHISPER_MODEL` = tiny|base|small|medium|large-v3, default `small` int8 on CPU. `stt_model` records the actual model. The 5 s target is measured with the default. The embedding model is not a runtime setting (changing it = new collection + alias swap). The GPU node pool is Future Work.
8. **Names.** Module path placeholder `github.com/framepulse/framepulse` stays; `gen/` committed in the monorepo.
9. **No auth/tenancy in MVP.** `ChunkRef` field 3 is held for `tenant_id`.
10. **Kubernetes-only testing.** Compose and testcontainers are dropped. Acceptance/BDD tests (godog) run on a local `kind` cluster, the same in CI, deploying the production Helm chart with `values-kind.yaml`. Unit tests per service run without infrastructure (decision 16, CI steps 6–9).
11. **Abandoned uploads.** Force-finalize after 5 min with no `HLS_INDEX` activity (`FINALIZE_IDLE_TIMEOUT`, default `5m`; sweeper every 30 s in each Stream Worker replica; `values-kind.yaml` uses `10s` with a `2s` sweep, see resolved decision 18). `n` = `expected_chunk_count` or highest seen seq + 1; missing/pending chunks are treated as FAILED; `finalize_reason = IDLE_TIMEOUT`. Zero READY chunks → finalized with no playlist, endpoints return 410. Late events never reopen a finalized video (late chunks counted in `fp_late_chunks_total`). New `HlsIndex` fields 6–7 above.
12. **Search shows unfinished videos** with a `status` field (`processing` / `ready` / `failed`).
13. **Language detection stays per chunk** in the MVP; per-video pinning is spec Future Work item 5.
14. **Video length cap (QA G2).** At most 4 h of media and 8,000 chunks per video; `Segment.key` dropped (field 2 reserved), so `HlsIndex` stays ≤ ~152 KB (B1 KV size budget). The Producer rejects longer files before any upload and rolls camera sessions over to a new `video_id`; workers dead-letter `chunk_seq ≥ 8000` or `chunk_count > 8000` as `ERROR_CLASS_SCHEMA`. Replaces the former 24 h session default (spec A13). Index sharding is spec Future Work 9.
15. **Point-ID namespace (QA G1).** `c1907ffe-0c06-4077-8866-08fb8d658b77`, fixed forever (B3, spec A18).
16. **DLQ cap (QA G3).** Dead-letter on the 5th delivery: at most 5 processing attempts; a transient failure on attempt 5 → `RETRIES_EXHAUSTED` + `Term()`; `num_delivered ≥ 6` → dead-lettered on receipt (B1 "DLQ mechanism", spec A19).
17. **Test fault hooks (QA G5).** `Fp-Test-Fault` header honored only with `FP_TEST_FAULTS=1`, set only by `values-kind.yaml` (B1 "Test-only fault hooks", spec 2.5 / A20).
18. **Idle timeout in tests (QA G6).** Production `5m` / sweep `30s`; `values-kind.yaml` `10s` / `2s`.
19. **Per-video filters (QA G4).** Four filter subjects across `VIDEO_EVENTS` and `VIDEO_DLQ` (B1 "Per-video filters").
