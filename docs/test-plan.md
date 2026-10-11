# FramePulse Test Plan

Status: draft by the QA agent, 2026-10-11. Acceptance criteria are the Gherkin features in
`framepulse_spec.md` section 4; contracts are `docs/architecture-review.md` Part B.
Testing is Kubernetes-only (spec A10): unit tests without infrastructure, acceptance tests
with godog on a local `kind` cluster, `@load` in dedicated runs only.

## 1. Layers

| Layer | Where | Infra | Runs | Gate |
|---|---|---|---|---|
| L0 Contract checks | `buf format/lint/breaking`, `buf generate && git diff` (B4 CI 1-4) | none | every PR | blocking |
| L1 Cross-language contract tests | each language's contracts lib vs `contracts/testdata/*.binpb` + `keys.json` (B4 CI 5) | none | every PR | blocking |
| L2 Unit tests per service | `go test ./...`, `cargo test`, `pytest` (B4 CI 6) | in-memory fakes, committed fixture media | every PR | blocking |
| L3 Test-harness unit tests | `make -C tests unit` (helpers + step coverage) | none (in-process nats-server) | every PR | blocking |
| L4 Acceptance (BDD) | `tests/acceptance`, Job on kind, `~@load` (B4 CI 8) | kind + Helm chart | every PR | blocking once steps implemented (`GODOG_STRICT=true`) |
| L5 Load | `@load` scenarios, `make -C tests load` | kind on a dedicated runner | nightly / on demand | indicative, trend-tracked |

### 1.1 L1: contract tests (B4 CI step 5)

`contracts/testdata/keys.json` should be the single source of truth for derived names. Each
entry: inputs (`video_id`, `chunk_seq`, `rendition`, `consumer`, `original_stream_seq`)
and expected outputs: subjects, the 5 `Nats-Msg-Id` forms plus the redrive form, all
object keys, the `HLS_INDEX` key, the Qdrant point ID, API paths, and the
`timestamp` string for `start_ms/end_ms` pairs. Required cases:
- `chunk_seq` 0, 5, 42, 999999; seq above 999999 must be rejected by every builder
  (the spec defines no 7-digit form);
- invalid `video_id`s (uppercase, v4, braces, trailing newline, `*`, `>`, `.`, `/`):
  validators reject them in all three languages;
- the point-ID namespace UUID (**not yet defined**, see G1);
- every `*.binpb` decodes in Go, Rust and Python; unknown fields and unknown enum values are
  preserved or tolerated (evolution rules).

The acceptance module's `internal/contracts` builders must also be tested against
`keys.json` once it exists (TODO in `contracts.go`).

### 1.2 L2: unit tests each service must have

**Producer (Go)**
- codec gate: H.264+AAC accepted; MPEG-4 Part 2, HEVC, VP9, Opus, MP3, and video without
  audio handled per decision (G9); corrupt/empty/non-media input rejected before any upload.
- segmenting: `-c copy` cut (no re-encode), 0-based contiguous seq, every chunk starts on
  an IDR, `media_time` measured from PTS (never assumed 2000 ms), last short chunk.
- ordering: raw PUT before publish; `VideoIngestCompleted` only after the last PubAck;
  sha256 computed over the uploaded bytes; Msg-Ids deterministic, so retries dedupe.
- failures: S3 PUT failure, NATS publish timeout and retry reuse the same `event_id` and
  Msg-Id; a resumed run after the 10 m duplicate window (G12).
- input path handling: symlinks, paths outside an allowed root if run as a service.

**Stream Worker (Rust)**
- classification table: every error maps to permanent or transient and to an `ErrorClass`;
  `num_delivered > 5` leads to the DLQ before processing; a failed DLQ publish NAKs and never
  Terms.
- ffprobe/ffmpeg subprocess: hard timeout, kill on drop, stderr capture truncated to 4 KiB,
  no shell interpolation of keys.
- HlsIndex CAS: concurrent updates (simulated revision conflicts), set semantics (a
  duplicate chunk does not add an entry), sorted insert, FAILED overrides pending, READY
  after FAILED (redrive) is defined (G15).
- finalization predicate: all combinations of expected count unknown/known, missing,
  pending, READY and FAILED; idempotent re-run; finalized index never reopened.
- playlist renderer (golden files): VOD header, `TARGETDURATION` = ceil(max EXTINF),
  EXTINF formatting/precision (G7), DISCONTINUITY before the first READY after any gap,
  never before the first segment, relative URIs only, byte-identical across runs.
- sweeper: idle detection with an injectable clock, `n` = expected or max seen + 1,
  zero-READY case writes no playlist, concurrent sweepers.
- side-effect order: outputs, then KV, then publish (await PubAck), then ack. Assert with
  fakes that a crash at each boundary leaves a redeliverable state.

**AI Worker / Embedder (Python)**
- prefixing: `query: ` / `passage: ` added exactly once; callers' text never pre-prefixed;
  `PURPOSE_UNSPECIFIED` rejected.
- golden-vector test vs reference sentence-transformers (cosine ≥ 0.999, spec 2.3).
- startup refusals: model not baked in; alias `transcripts` pointing at another collection.
- outcomes: speech gives INDEXED; silence gives NO_SPEECH (transcript written, no point);
  no audio stream gives NO_AUDIO (no transcript, no point); transcripts with non-UTF-8 or
  control characters are sanitised before the proto3 JSON is written.
- point ID = UUIDv5(namespace, `{video_id}:{seq6}`) vs `keys.json`; payload fields and
  types; `thumbnail_key` from the fixed pattern without waiting.
- timeouts: `AI_PIPELINE_TIMEOUT`, `AckProgress` every 15 s; Qdrant down leads to a NAK with
  2/10/30/60 s delays.

**Serving API (Go, Gin)**
- playlist state machine with a fake KV and fake S3: 404 (no key, unknown rendition), 409
  (body, headers, `expected_chunk_count: null`), 200 (verbatim bytes, headers), 410, 503.
- input validation table (security, see section 4): `video_id`, `rendition`, `seq6`, `q`,
  `limit`.
- presigning: public endpoint host, 10 min TTL, path-style key, GET only;
  `Cache-Control` on the 302 (G13).
- search: merge algorithm (gap ≤ 1, max score, joined snippet, best chunk's thumbnail),
  floor/ceil timestamps, status join from `HLS_INDEX`, embedder timeout/unavailable leads
  to 503, model mismatch is rejected.

### 1.3 L4: acceptance on kind

- Harness: `tests/acceptance` (godog), one feature file per spec feature copied verbatim.
  `TestFeaturesMatchSpec` fails if they drift.
- Every scenario gets fresh UUIDv7 video IDs (aliases `V`, `W`), so scenarios don't share
  objects or KV keys. Search is global, though (G10).
- "Then" steps wait on an ordered JetStream consumer per video (VIDEO_EVENTS +
  VIDEO_DLQ) or a KV watch on `HLS_INDEX`, never on sleeps.
- Fault injection: `kubectl scale` with LIFO restore in the scenario After hook and an
  AfterSuite safety net; KEDA-managed workers are pinned with
  `autoscaling.keda.sh/paused-replicas`, or KEDA would scale them back up.
- Scenarios run serially (concurrency 1 is enforced): fault injection and global search
  results make parallel runs flaky.
- Duration budget: the three idle-timeout scenarios wait ≥ 5.5 min each with the spec
  default; see G6.

### 1.4 L5: load (`@load`)

- Hot path: 5 Producer processes × 60 chunks paced at 1 chunk/2 s; p95 publish-to-READY
  from stream timestamps (uploaded event, then the KV revision that marks READY).
  p99 publish-to-delivery cannot be measured outside the worker, so it comes from the
  `fp_consume_latency_seconds` histogram (G8).
- Cold path: 50 speech chunks one at a time, `ai-indexer` with no backlog, p95 from stream
  timestamps. With nearest-rank, p95 of 50 is the 48th sample.
- Record results (summary line per run) and trend them; results are indicative (B4 CI 9).

## 2. Traceability matrix

F = feature file in `tests/acceptance/features/`. Services: P Producer, SW Stream Worker,
AI AI Worker, E Embedder, API Serving API, N NATS, M MinIO, Q Qdrant, K kube.

| # | Scenario | F | Key assertions | Services |
|---|---|---|---|---|
| 1.1 | Valid video becomes HLS segments and plays as VOD | 01_hot_path | raw object + `video/mp2t`; uploaded/hls-ready events with Msg-Ids; segment is a remux (packet hashes equal); thumbnail; KV READY + measured duration; finalized; playlist 200 VOD+ENDLIST, byte-identical to object; EXTINF = measured; segment 302 to presigned public URL ≤ 10 min | P SW API N M |
| 1.2 | @load hot-path latency | 01_hot_path | p99 delivery ≤ 500 ms (worker histogram); p95 publish-to-READY ≤ 1.5 s (≥ 300 samples) | P SW N M |
| 1.3 | Out-of-order chunks | 01_hot_path | KV lists 0,1,2 sorted, once each; static playlist order 0,1,2 | SW N M |
| 1.4 | Corrupted chunk dead-lettered | 01_hot_path | DLQ on `dlq.hls-stream-worker.V`, INVALID_MEDIA, redrivable=false, delivery_count 1; no redelivery; chunk 3 FAILED; chunks 4,5 READY; no pod restarts; playlist omits 000003.ts, DISCONTINUITY before 000004.ts | SW N M K |
| 2.1 | Playlist before finalization | 02_finalization | 409, JSON, `Cache-Control: no-store`, `Retry-After: 5`; body counts and `null`; master same; unknown UUIDv7 gives 404 | SW API N |
| 2.2 | VideoIngestCompleted finalizes | 02_finalization | playlist 5 segments VOD+ENDLIST; master one variant; finalized only after both objects exist; 200 proxied (no Location); variant resolves to API route | SW API N M |
| 2.3 | Completion before last chunk | 02_finalization | no playlist after completion is recorded; 409 with expected 5; written once after chunk 4 READY; 200 | SW API N M |
| 2.4 | Idle-timeout finalization | 02_finalization | reason IDLE_TIMEOUT; playlist 0,1,3 with DISCONTINUITY before 3; ENDLIST; 200 | SW API N M |
| 2.5 | No playable chunk | 02_finalization | finalized IDLE_TIMEOUT, no playlist objects; 410 `{"status":"failed"}` | SW API N M |
| 2.6 | Late chunk | 02_finalization | playlist bytes and KV revision unchanged; segment still written; `fp_late_chunks_total` +1 (summed over replicas) | SW N M K |
| 3.1 | Speech chunk indexed | 03_cold_path | stt_model `faster-whisper/small/int8`; language en; transcript key; passage prefix (vector compare); point UUIDv5 in alias→collection; payload thumbnail_key/language/start/end; event INDEXED with model + physical collection | AI E N M Q |
| 3.2 | No speech / no audio | 03_cold_path | NO_SPEECH: no point; NO_AUDIO: no transcript, no point | AI N M Q |
| 3.3 | @load cold-path latency | 03_cold_path | no backlog; p95 ≤ 5 s over 50 | AI N M Q |
| 4.1 | AI failure retried then DLQ | 04_failures | Qdrant scaled to 0; growing NAK delays; DLQ RETRIES_EXHAUSTED; original subject/headers/payload verbatim; hot path unaffected | AI SW N Q K |
| 4.2 | Crash loop broken | 04_failures | delivered > 5 leads to DLQ before processing, Term | AI N K (needs hook, G5) |
| 4.3 | Redrive | 04_failures | republished with `{orig}:redrive:n`; each chunk indexed exactly once | fpctl AI N Q |
| 5.1 | Duplicate publish deduped | 05_idempotency | second PubAck duplicate; StoredCount(subject)=1 | N |
| 5.2a | Redelivery, Stream Worker | 05_idempotency | same keys/equivalent bytes; KV chunk 5 once; one ChunkHlsReady | SW N M (needs hook, G5) |
| 5.2b | Redelivery, AI Worker | 05_idempotency | same transcript key; one point; one ChunkTranscriptIndexed | AI N M Q (needs hook, G5) |
| 6.1 | Merged ranges | 06_search | EmbedRequest QUERY unprefixed; result fields table; one merged range | API E Q N |
| 6.2 | Processing videos shown | 06_search | W status processing; playback_url 409 then 200 | API Q N |
| 6.3 | Thumbnail redirect | 06_search | 302, public endpoint, TTL ≤ 10 min | API M |
| 6.4 | Thumbnail placeholder | 06_search | placeholder image, never 4xx/5xx (status undefined, G11) | API M |
| 6.5 | Legacy path | 06_search | `/api/search` JSON-equal to `/api/v1/search` | API |
| 7.1 | Retention | 07_retention | 3 streams max_age 7 d; fp-raw expiry 10 d; fp-media none; 10 d > 7 d | N M |

Harness status (2026-10-11): all 25 scenarios (149 steps, 120 definitions) defined and
pending; `make -C tests acceptance-dry` passes; `GODOG_STRICT=true` fails on pending as intended.

## 3. Spec gaps, contradictions and risks

Severity is how likely the gap is to cause a wrong implementation or a test that can't be
written.

| ID | Sev | Gap | Where | Proposal |
|---|---|---|---|---|
| G1 | High | Qdrant point-ID UUIDv5 **namespace is never defined**; Go/Rust/Python and the tests cannot agree on point IDs | B3 | Pin a namespace UUID in `contracts/testdata/keys.json` and B3. Tests use a placeholder + `FP_POINT_ID_NAMESPACE` |
| G2 | High | `HLS_INDEX` value can exceed `max_value_size` 1 MiB (and default NATS `max_payload` 1 MiB) within the 24 h session limit (A13): 43,200 segments × ~85 B (key string ~61 B + seq + TimeRange + status) ≈ 3.7 MB; the 1 MiB limit is hit after ~12k chunks (~6.8 h). Each CAS also rewrites the whole value (O(n²) bytes per video) | B1 KV, A13 | Drop the derivable `Segment.key`, store media_time as duration, and/or shard the index (`{id}.{rendition}.{block}`); or cap sessions at ~6 h. Add a unit test rendering a 43,200-segment index |
| G3 | Med | DLQ delivery cap is inconsistent: Feature 4 says "after the 5th delivery publishes a DeadLetter"; B1 step 1 says on receipt when `num_delivered > 5` (the 6th delivery); B1 step 3 says "on the 5th attempt, step 1" | spec F4, B1 | Pick one: "a transient failure on delivery 5 dead-letters immediately; a receipt with num_delivered > 5 dead-letters before processing". Assert `delivery_count` exactly |
| G4 | Med | Per-video subject filter: `video.chunks.*.{id}` (as one might write it) only matches `uploaded`, because `hls.ready`/`ai.indexed` have 5 tokens | B1 subjects | Document that per-video filters need `video.chunks.*.{id}` + `video.chunks.*.*.{id}`, or flatten to `video.chunks.hls_ready.{id}`. Harness uses both filters (`contracts.EventFilterSubjects`, tested) |
| G5 | High (testability) | "Crash before ack" (F4.2) and "fully processed but not acked" (F5.2) can't be produced black-box without races | F4, F5 | Add test-only fault hooks enabled only by `values-kind.yaml`, e.g. `FP_TEST_FAULTS=crash_before_ack:source_id=…`/`skip_ack:…`, keyed on `VideoChunkUploaded.source_id`. Must be compiled out or rejected in prod values |
| G6 | Med | Idle-timeout scenarios hard-code `FINALIZE_IDLE_TIMEOUT "5m"`; 3 scenarios cost ≥ 17 min per PR run | F2 | values-kind sets e.g. `30s` (sweep `5s`); make the step "runs with the configured FINALIZE_IDLE_TIMEOUT" and assert relative to it |
| G7 | Med | EXTINF precision/rounding undefined ("equal to the measured duration") | F1, 2.2 | Define: milliseconds, 3 decimals, `%.3f` of `(end_ms-start_ms)/1000`; TARGETDURATION = ceil of that value |
| G8 | Med | p99 publish-to-delivery is not observable outside the worker; histogram bucket granularity limits p99 accuracy | F1 @load, A4 | Define the `fp_consume_latency_seconds` buckets (incl. 0.25/0.5/0.75 s) and that the load test reads the histogram delta |
| G9 | Med | Producer handling of H.264 **without audio** (common for cameras) is undefined; A12 says H.264+AAC required, but NO_AUDIO exists as a cold-path outcome | A12, F3 | Decide: accept video-only (NO_AUDIO path) or reject. Fixture `sample_h264_noaudio.mp4` is ready |
| G10 | Med | Search is global with no `video_id` filter; other scenarios' videos (same speech fixture) compete for top-N, so Feature 6 can be flaky | F6, B3 | Allow `limit` up to 50 in tests and filter by video client-side; or add an internal `video_id` filter param |
| G11 | Low | Placeholder thumbnail response undefined (200 image vs 302 to a placeholder) and its caching | F6.4, 2.1 | Specify: `200 image/jpeg` (or `image/svg+xml`), `Cache-Control: no-store` |
| G12 | Low | Dedup window 10 m: a Producer re-run/resume after 10 m duplicates stored events (outputs stay idempotent, but counts and "exactly one" assertions break) | B1 | Document; the Producer should check `HLS_INDEX`/stream before re-publishing on resume |
| G13 | Med | Caching of the API's 302 responses is undefined; if cached longer than the 10 min presign TTL, players get expired URLs | 2.1, A5 | Require `Cache-Control: private, max-age ≤ 60` (or `no-store`) on 302s |
| G14 | Low | `playback_url` "start-time hint" format undefined | F6.1 | Define, e.g. `…/master.m3u8#t=85` or `?start_ms=85000` |
| G15 | Med | Redrive after a hot-path DLQ: a chunk marked FAILED that later becomes READY via redrive (e.g. OBJECT_NOT_FOUND is redrivable): does READY replace FAILED, and what if the video is already finalized? | 2.2, B1 redrive | Define precedence; after finalization it is a late chunk (counted, no reopen) |
| G16 | Low | `VideoIngestCompleted.chunk_count` smaller than the highest seen seq, zero, or a second completion with a different count: undefined | 2.2 | Define: chunks ≥ count are ignored for the playlist and logged; count 0 is invalid, so DLQ SCHEMA |
| G17 | Low | Feature 6 Background: chunks 42..49 "spanning 85.000 s to 100.000 s" (15 s for 8 chunks) conflicts with ~2 s chunks (42×2 = 84 s); expected `timestamp` depends on the fixture | F6 | Make the Background derive the range from the indexed chunks, or say the fixture must produce exactly these media times |
| G18 | Low | F6 scenarios 3 and 4 refer to "the result" but contain no search step | F6 | Add `When the user searches for "architecture"` to both (step currently searches implicitly) |
| G19 | Low | F2.3 "chunk 4 is still being processed" can't be held from outside | F2 | Reword to "the event for chunk 4 has not been published yet" |
| G20 | Low | Where 410 applies is inconsistent: spec 2.1/2.2 put 410 on the playlist endpoints; B2 lists it under the segment/thumbnail routes | 2.1, B2 | Define 410 for playlists (and segment routes?) explicitly |
| G21 | Low | "Different replicas" (F1.3) is not observable on success | F1 | Require `Fp-Producer`/`worker_instance` on `ChunkHlsReady` (e.g. header `Fp-Instance`) |
| G22 | Low | `fp_late_chunks_total` is per pod; restarts reset it, so "incremented" can be falsely negative | F2.6 | Assert via sum over replicas within the scenario, with no pod restarts; or emit a `VideoLateChunk` event |
| G23 | Low | Finalization emits no event; tests must watch KV | 2.2 | Optional `video.lifecycle.finalized.{id}` event (also useful for search re-ranking / notifications) |
| G24 | Low | `fpctl dlq redrive --since 1h` redrives every dead letter in the window, including other tests' | F4.3 | Add `--video-id`/`--subject` filters; run redrive scenarios in isolation |
| G25 | Low | In-cluster runner vs `MINIO_PUBLIC_ENDPOINT`: presigned URLs signed for `localhost:9000` can't be fetched from the Job pod (signature binds Host) | A5, kind | Use a public endpoint that resolves both in cluster and on host (e.g. `minio.127.0.0.1.nip.io` + CoreDNS rewrite) or only assert the URL shape in-cluster |
| G26 | Low | Cold-path handling of corrupt/missing/checksum-mismatched chunks is not specified as scenarios (only hot path) | F1, F4 | Add scenarios (below) |

## 4. Security tests to add (Serving API, workers)

No auth in the MVP (A9), so input validation is the main control. Proposed acceptance
scenarios (plan only; the feature files stay verbatim) and matching API unit tables:

1. **video_id validation**: `/api/v1/videos/{id}/…` with `..`, `%2e%2e`, `..%2F..%2Ffp-raw`,
   uppercase UUID, UUIDv4, 37+ chars, `*`, `>`, `.`, NUL/`%00`, newline. Expect 400 or 404,
   never 5xx, and never a MinIO or KV lookup with that string (KV key `{id}.{rendition}`
   contains `.`; an id containing `.` or a wildcard could address another key or a KV
   wildcard).
2. **rendition validation**: only `src`; `../hls`, `src%2F..`, `master` give 404.
3. **seq6 validation**: `/hls/src/{seq6}.ts` with `1.ts`, `0000001.ts`, `-00001.ts`,
   `000001.ts%00.jpg`, `../../x.ts` give 404/400; presigned key is always rebuilt from
   validated parts, never from the raw path.
4. **Presign scope**: Location is GET-only, single object, TTL ≤ 600 s, public host; no
   `uploadId`/list parameters (helper `CheckPresignedURL` already enforces this);
   presigning for a non-existent object of an unknown video returns 404, not a signed URL
   (prevents bucket probing).
5. **Search input bounds**: `q` empty gives 400; `q` of 10 KB gives 400/414 (define a max,
   e.g. 512 chars) and must not reach the embedder; `limit` 0, -1, 1000000, `abc`
   give 400 or clamp to ≤ 50; non-UTF-8 `q` (`%ff`) gives 400; Unicode ja/zh queries work.
6. **Response safety**: transcript text in `snippet` is JSON-escaped; UI must render it as
   text (XSS via spoken/odd transcript text); error bodies never echo raw input unescaped.
7. **Resource limits**: API request timeouts with Qdrant/embedder down (no goroutine/conn
   leak; 503 within N s); playlist proxy body size bounded; Producer rejects files above a
   size/duration limit (24 h, A13).
8. **Secrets**: no credentials in logs/events/DLQ `error_message` (MinIO keys, presigned
   signatures); `DeadLetter.original_headers` must not carry auth headers.
9. **Event payload trust**: workers must not trust `ObjectRef.bucket/key` from events
   blindly. A `VideoChunkUploaded` whose `raw_object` points to another video's key, to
   `fp-media`, or to `../` gives DLQ SCHEMA (key must equal the derived key). This is also
   the SSRF-style risk: events must never carry endpoints/URLs (ObjectRef design note).
10. **Bucket exposure**: anonymous GET on MinIO buckets is denied (MinIO private);
    `fp-raw` not presignable via the API.

## 5. Proposed extra scenarios (not in feature files)

- Producer rejects `sample_mpeg4_aac.mp4` with a clear error and publishes nothing (A12).
- Producer behaviour for `sample_h264_noaudio.mp4` (after G9 is decided).
- Checksum mismatch (raw object replaced after publish) gives DLQ CHECKSUM_MISMATCH,
  redrivable=false, on both consumers.
- Raw object missing gives DLQ OBJECT_NOT_FOUND, redrivable=true; redrive after re-upload
  is READY.
- Unknown `Fp-Schema` / undecodable payload gives DLQ `ERROR_CLASS_SCHEMA` on
  `dlq.{consumer}.unknown` when video_id can't be read.
- Empty (0-byte) and truncated chunks are INVALID_MEDIA, never a crash; ffprobe timeout
  gives ERROR_CLASS_TIMEOUT.
- Corrupted chunk on the cold path: `dlq.ai-indexer.V` INVALID_MEDIA; hot path for the
  other chunks unaffected.
- Embedder unavailable: search returns 503 within the timeout; playlists unaffected.
- MinIO unavailable briefly: workers NAK (transient) and recover without DLQ.
- NATS restart mid-run: consumers resume; no duplicates (Msg-Id).
- Stream Worker pod deleted during finalization: another replica finalizes; playlists
  byte-identical.
- Two replicas finalizing concurrently (scale to 3, burst completion): one KV revision
  with finalized=true; identical playlist bytes.
- Search with Japanese/Chinese query returns results (multilingual model smoke test).
- Search result for a `failed` video: status `failed`, playback_url gives 410.
- Very long video: Producer on a multi-hour synthetic input (load lane) checks `HLS_INDEX`
  size against G2.
- Retention: `deny_delete` on VIDEO_EVENTS/VIDEO_DLQ, `discard: new` on VIDEO_DLQ,
  duplicate windows, consumer configs (ack_wait, max_deliver 10, max_ack_pending) match
  `contracts/nats/*.json`.

## 6. Open items for the developer

- `gen/go` + a `ProtoCodec` in `tests/acceptance/internal/payload` (replace `PendingCodec`).
- Producer CLI: `--video-id` flag (or machine-readable output of the assigned id) so
  tests can watch events before publishing. Ship the Producer and `fpctl` binaries into the
  runner image (TODO stages in the Dockerfile).
- Helm chart service/secret names; then update `tests/kind/acceptance-job.yaml`,
  `port-forward.sh` and the `FP_WORKLOAD_*` defaults.
- Test-only fault hooks (G5) gated to `values-kind.yaml`.
- Speech fixture recording (tests/fixtures/README.md).
