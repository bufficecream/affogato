# Stream Worker milestones (SW1–SW5)

Rust service on the hot path. Consumer `hls-stream-worker` receives `VideoChunkUploaded` and
`VideoIngestCompleted`, remuxes each chunk to an HLS segment (`-c copy`), grabs a thumbnail,
tracks per-segment state in KV `HLS_INDEX`, and writes the final VOD playlists when the video
is finalized.

Source of truth: spec §2.2, §2.5, A15, A19, A20; [`docs/architecture-review.md`](../../docs/architecture-review.md)
decisions 3, 4, 6, 7, 10 and B1/B2; required tests in [`docs/test-plan.md`](../../docs/test-plan.md)
§1.2 "Stream Worker". Uses `libs/rust/af-contracts` and `gen/rust` (C4).

Stack: `tokio`, `async-nats`, `aws-sdk-s3` (path-style), `prost`, ffmpeg/ffprobe CLI as
subprocesses. Unit tests use in-memory fakes for NATS, KV and S3; ffmpeg tests use
`tests/fixtures/media`.

---

## SW1. Consumer loop, DLQ rules and fault hooks

**Status:** planned · **Depends on:** C4

**Scope**
- Config from env (NATS, S3, `FINALIZE_IDLE_TIMEOUT`, `FINALIZE_SWEEP_INTERVAL`,
  `AF_TEST_FAULTS`), with startup checks that the stream, consumer and KV exist (infra owns
  them; the worker never creates them).
- A pure decision function, tested as a table, for the B1 "DLQ mechanism":
  - permanent vs transient error mapped to an `ErrorClass`;
  - transient on attempts 1–4 → NAK with 1 s / 2 s / 5 s / 15 s;
  - transient on attempt 5 → `DeadLetter` `RETRIES_EXHAUSTED`, `delivery_count` 5, `Term()`;
  - `num_delivered` ≥ 6 on receipt → dead-letter without processing;
  - a failed DLQ publish → NAK, never `Term()`.
- Schema validation: `chunk_seq` ≥ 8000 or `chunk_count` > 8000 → `ERROR_CLASS_SCHEMA`,
  nothing written.
- Test-only fault hooks (B1, spec 2.5): `AF_TEST_FAULTS` unset/empty/`0` ignores the header,
  `1` honours it, anything else fails startup; `exit-on-receive`, `crash-before-ack`,
  `fail-transient`; malformed values dead-lettered; never copied onto result events.

**Verify**
```sh
cargo fmt --check --manifest-path services/stream-worker/Cargo.toml
cargo clippy --manifest-path services/stream-worker/Cargo.toml -- -D warnings
cargo test --manifest-path services/stream-worker/Cargo.toml dlq:: fault:: schema::
```

**Report includes:** the decision table (attempt × error type → action) as tested.

---

## SW2. Remux, probe, thumbnail and object writes

**Status:** planned · **Depends on:** SW1

**Scope**
- Download the raw chunk, verify sha256 (mismatch → `CHECKSUM_MISMATCH`).
- ffprobe validation, then remux with `-c copy`. ffprobe alone is not enough:
  `chunk_truncated.ts` passes ffprobe and only the remux fails, so both must run.
- Thumbnail 320×180 from the first IDR → `af-media/videos/{vid}/thumbs/{seq6}.jpg`.
- Segment → `af-media/videos/{vid}/hls/src/{seq6}.ts` with the B2 content type and cache headers.
- Every subprocess: hard timeout (10 s), kill on drop, stderr capped at 4 KiB, no shell
  interpolation of keys.
- Corrupt, truncated or empty input → `INVALID_MEDIA` (permanent).

**Verify**
```sh
cargo test --manifest-path services/stream-worker/Cargo.toml media::
```
With fixtures: `chunks/six/*.ts` remux with packet hashes equal to the input;
`chunk_corrupted.ts`, `chunk_truncated.ts` and `chunk_empty.ts` → `INVALID_MEDIA`;
`chunk_no_audio.ts` still produces a segment; a fake slow ffmpeg is killed at the timeout.

**Report includes:** remux time per fixture chunk (first data point for the 1.5 s p95 budget).

---

## SW3. `HLS_INDEX` updates and `ChunkHlsReady`

**Status:** planned · **Depends on:** SW2

**Scope**
- CAS update of `{video_id}.src`: sorted insert by `chunk_seq`, set semantics (a duplicate
  chunk adds nothing), `last_activity_at` stamped on every update, FAILED written in the same
  step that dead-letters the chunk, READY after FAILED (redrive) handled as decided for
  test-plan G15.
- Publish `ChunkHlsReady` with its deterministic Msg-Id and await the PubAck.
- Side-effect order: objects → KV → publish (PubAck) → ack. Tests inject a crash at each
  boundary and show the redelivery reaches the same final state (Feature 5.2a).
- `HlsIndex` with 8,000 segments encodes to ≤ 160 KB.

**Verify**
```sh
cargo test --manifest-path services/stream-worker/Cargo.toml index:: pipeline::
```

**Report includes:** the CAS conflict and crash-boundary test cases; the measured worst-case
index size.

---

## SW4. Finalization, playlists and idle sweeper

**Status:** planned · **Depends on:** SW3 · **Unblocks:** ACC2 (with P3, API2, H2)

**Scope**
- Finalization predicate: `expected_chunk_count` known (from `VideoIngestCompleted`) and
  every chunk READY or FAILED; idempotent; a finalized index is never reopened; late chunks
  are still written but counted in `af_late_chunks_total` and do not change the playlist.
- Playlist renderer (golden-file tests): VOD header, `#EXT-X-PLAYLIST-TYPE:VOD`,
  `TARGETDURATION` = ceil(max EXTINF), EXTINF precision (G7), `#EXT-X-DISCONTINUITY`
  before the first READY segment after a gap (never before the first segment), relative URIs,
  `#EXT-X-ENDLIST`, byte-identical across runs. `master.m3u8` with one variant.
- Order: PUT both playlists, then CAS `finalized=true` with `finalize_reason`.
- Idle sweeper (spec A15): every `FINALIZE_SWEEP_INTERVAL`, finalize indexes idle for
  `FINALIZE_IDLE_TIMEOUT` with `n` = expected or highest seen + 1, missing chunks as FAILED,
  reason `IDLE_TIMEOUT`; zero READY chunks → finalized with no playlists. Injectable clock;
  concurrent sweepers in several replicas are safe.

**Verify**
```sh
cargo test --manifest-path services/stream-worker/Cargo.toml finalize:: playlist:: sweeper::
```
Golden files under `services/stream-worker/testdata/playlists/` cover: in order,
out of order, a FAILED gap (chunk 3 of 6), idle timeout with missing chunks, zero READY.

**Report includes:** the golden-file list; the G7 and G15 resolutions.

---

## SW5. Metrics, image and latency budget

**Status:** planned · **Depends on:** SW4

**Scope**
- `/metrics` on `AF_STREAM_WORKER_METRICS_PORT` (9090): `af_consume_latency_seconds`
  (publish-to-delivery histogram), `af_late_chunks_total`, processing outcome counters; health
  and readiness endpoints for the chart.
- Container image `affogato/stream-worker:dev` (multi-stage; ffmpeg copied from the pinned
  `linuxserver/ffmpeg` image used by the fixtures, no apt download), non-root.
- A micro-benchmark of one chunk end to end against fakes, to compare with the 1.5 s p95
  segment-ready target before the `@load` run (ACC4).

**Verify**
```sh
cargo test --manifest-path services/stream-worker/Cargo.toml
docker build -t affogato/stream-worker:dev services/stream-worker
docker run --rm affogato/stream-worker:dev --version
```

**Report includes:** image size, benchmark p50/p95 per fixture chunk.

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
