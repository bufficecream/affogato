# Producer milestones (P1–P4)

Go CLI `af-producer`: probes an `.mp4`, cuts it into ~2 s MPEG-TS chunks without re-encoding,
uploads each chunk to `af-raw` and publishes `VideoChunkUploaded`, then
`VideoIngestCompleted`. This module also holds `afctl`, the dead-letter redrive tool (decision
D4 in [`docs/implementation-plan.md`](../../docs/implementation-plan.md)).

Source of truth: spec §2.1 and A12/A13, [`docs/architecture-review.md`](../../docs/architecture-review.md)
decision 2 and B1/B2, required tests in [`docs/test-plan.md`](../../docs/test-plan.md) §1.2
"Producer". Uses `libs/go/afcontracts` and `gen/go` (C4).

Layout: `services/producer/{cmd/af-producer,cmd/afctl,internal/...}`, one Go module.
Unit tests use fakes for S3 and NATS and the fixtures in `tests/fixtures/media`; they need a
host ffmpeg/ffprobe or run through `tests/ffmpeg-docker.sh` (`AF_FFPROBE_BIN`/`AF_FFMPEG_BIN`).

---

## P1. Probe, codec gate and length cap

**Status:** planned · **Depends on:** C4, decision D2 (H.264 without audio)

**Scope**
- `internal/probe`: runs ffprobe as a subprocess (hard timeout, no shell, stderr capped at
  4 KiB) and returns codecs, profile, duration and key-frame times.
- Codec gate (A12): H.264 + AAC accepted; MPEG-4 Part 2, HEVC, VP9, Opus, MP3 rejected;
  H.264 without audio handled as decided in D2; corrupt, empty and non-media input rejected.
- Length cap (A13): longer than 4 h or more than 8,000 chunks rejected; exactly 4 h / 8,000
  accepted. Tested with a fake prober (no 4-hour fixture).
- Every rejection happens **before any upload or publish** (asserted with the fakes).
- Clear CLI errors and exit codes, listed in `services/producer/README.md`.

**Verify**
```sh
(cd services/producer && go vet ./... && go test ./internal/probe/... ./internal/gate/...)
```
Table tests use `sample_1080p.mp4` (accept), `sample_mpeg4_aac.mp4` (reject),
`sample_h264_noaudio.mp4` (D2), `chunk_corrupted.ts` and `chunk_empty.ts` (reject).

**Report includes:** the accept/reject table with the reason shown for each fixture.

---

## P2. Segmenter

**Status:** planned · **Depends on:** P1

**Scope**
- `ffmpeg -c copy -f segment -segment_format mpegts -segment_time 2 -segment_time_delta 0.1`
  (the delta is required; without it ffmpeg 7.1.1 misses the first cut on B-frame input).
- Chunks are 0-based and contiguous, every chunk starts on an IDR, `media_time` is measured
  from PTS (never assumed 2000 ms), and the last chunk may be short.
- No re-encode: packet hashes of the chunks match the source packets.

**Verify**
```sh
(cd services/producer && go test ./internal/segment/...)
```
- `sample_12s.mp4` gives exactly 6 chunks matching `tests/fixtures/media/chunks/six/*.ts`
  (same packet count, 60 video packets each, IDR first).
- `sample_short.mp4` gives a short last chunk with its measured duration.

**Report includes:** chunk count and measured durations per fixture.

---

## P3. Upload, publish and completion

**Status:** planned · **Depends on:** P2 · **Unblocks:** ACC2 (with SW4, API2, H2)

**Scope**
- Per chunk: PUT `af-raw/videos/{vid}/chunks/{seq6}.ts` (`video/mp2t`, `x-amz-meta-af-*`,
  sha256 over the uploaded bytes), **then** publish `VideoChunkUploaded` with the
  deterministic `Nats-Msg-Id` and await the PubAck.
- `VideoIngestCompleted` (with `chunk_count`) only after the last chunk's PubAck.
- Retries reuse the same `event_id` and Msg-Id, so a retried publish deduplicates (Feature 5.1).
- Failures: S3 PUT failure, publish timeout, resume after the 10 min duplicate window
  (test-plan G12; the resolution is written in the report).
- Camera sessions (A13): at the 8,000-chunk cap, complete the current video and continue under
  a new UUIDv7 `video_id` with media time restarting at 0 (tested with fakes).
- Config via flags/env: NATS URL and creds, S3 endpoint, credentials, path-style.

**Verify**
```sh
(cd services/producer && go test ./...)
```
Manual smoke test on kind once H1 is up (port-forwarded NATS and object store): upload
`sample_1080p.mp4`, then show 5 raw objects and 5 + 1 events with `nats stream view`.
Optional large-input check with the local, git-ignored `tests/data/The Truman Show.mp4`
(probe first; rejected cleanly if it is not H.264 + AAC).

**Report includes:** the side-effect order proven by the fake (PUT → publish → PubAck →
completion); the retry/dedupe result; smoke-test output once a cluster exists.

---

## P4. Binaries for the runner image (`af-producer`, `afctl`)

**Status:** planned · **Depends on:** P3, D4

**Scope**
- `afctl dlq redrive --consumer X --since T`: republishes `original_payload` to
  `original_subject` with the original headers minus `Af-Test-Fault`, Msg-Id
  `{orig}:redrive:{n}` (B1 "Redrive"). Also `afctl dlq list` for inspection.
- Fill in the two TODO build stages in `tests/acceptance/Dockerfile`, so the runner image
  contains `af-producer` and `afctl` (`AF_PRODUCER_BIN`, `AF_AFCTL_BIN`).
- A service image is not needed: the Producer runs as a CLI (in the runner Job on kind).

**Verify**
```sh
(cd services/producer && go test ./cmd/afctl/... ./internal/redrive/...)
make -C tests image && docker run --rm --entrypoint sh affogato/acceptance:dev -c 'af-producer --version && afctl --version'
```

**Report includes:** redrive behaviour proven against an in-process NATS server (headers,
Msg-Id, fault header dropped).

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
