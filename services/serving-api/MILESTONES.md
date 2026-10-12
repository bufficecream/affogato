# Serving API milestones (API1–API4)

Go (Gin) HTTP API. Serves the final VOD playlists (proxied from object storage), redirects
segments and thumbnails to presigned URLs, and answers `/api/v1/search` using the Embedder and
Qdrant.

Source of truth: spec §2.4 and Feature 2 / Feature 6; [`docs/architecture-review.md`](../../docs/architecture-review.md)
decisions 4, 9, 14 and B2 "Serving API exposure", B3 "Search"; required tests in
[`docs/test-plan.md`](../../docs/test-plan.md) §1.2 "Serving API" and §4 (security tests).
Uses `libs/go/afcontracts` and `gen/go` (C4).

Unit tests use fakes for KV, S3, the Embedder and Qdrant (`httptest` for HTTP).

---

## API1. Skeleton, validation and presigning

**Status:** planned · **Depends on:** C4

**Scope**
- Gin server, config from env, `/healthz` and `/readyz`, structured logs, request IDs.
- Input validation table (test-plan §4): `video_id` (UUIDv7 only), `rendition`, `seq6`,
  `q` (length cap), `limit` (range). Invalid input → 400 with a JSON error, never a 500.
- Presign helper: GET only, 10 min TTL, signed against `MINIO_PUBLIC_ENDPOINT`, path-style
  keys, `Cache-Control` on the 302 (test-plan G13).

**Verify**
```sh
(cd services/serving-api && go vet ./... && go test ./internal/validate/... ./internal/presign/...)
```

**Report includes:** the validation table (input → status) as tested.

---

## API2. Playlist state machine and media redirects

**Status:** planned · **Depends on:** API1 · **Unblocks:** ACC2 (with P3, SW4, H2)

**Scope**
- `GET /api/v1/videos/{id}/hls/master.m3u8` and `/hls/{rendition}/playlist.m3u8`:
  - 404: no `HLS_INDEX` key or unknown rendition;
  - 409: not finalized, `Cache-Control: no-store`, `Retry-After: 5`, JSON body with
    `segments_ready`, `segments_failed`, `expected_chunk_count` (or `null`);
  - 200: finalized, object bytes proxied verbatim (no redirect), `max-age=86400`;
  - 410: finalized with no playable segment, `{"video_id","status":"failed"}`;
  - 503: finalized but playlist object missing (invariant violation, logged as an alert).
- `GET …/hls/{rendition}/{seq6}.ts` and `GET …/thumbs/{seq6}.jpg` → 302 to presigned URL.
- Thumbnail fallback placeholder image when the object is missing (Feature 6.4, G11).

**Verify**
```sh
(cd services/serving-api && go test ./internal/playlist/... ./internal/media/...)
```
Fake-backed tests for every row above, including byte-identical proxying of a golden playlist
from SW4.

**Report includes:** the state table as tested; header sets per response.

---

## API3. Search

**Status:** planned · **Depends on:** API2, AI1 (Embedder contract), D3

**Scope**
- Embedder client: NATS request-reply on `svc.embed.v1.embed`, `PURPOSE_QUERY`, unprefixed
  text, timeout → 503; reject a response whose `model` differs from the collection's model.
- Qdrant query on alias `transcripts`, top 50.
- Merge (B3): group by video, merge consecutive `chunk_seq` (gap ≤ 1), max score, joined
  snippet, best chunk's thumbnail; timestamps floor/ceil to the second, `HH:MM:SS - HH:MM:SS`.
- Result fields: `video_id, start_ms, end_ms, timestamp, snippet, thumbnail_url, playback_url,
  status, score`; `status` from `HLS_INDEX` (`processing` / `ready` / `failed`), results never
  hidden by status.
- Legacy alias `/api/search` returns the same JSON as `/api/v1/search`.

**Verify**
```sh
(cd services/serving-api && go test ./internal/search/...)
```
Table tests for the merge (single chunk, adjacent, gap of 1, gap of 2, several videos, ties).

**Report includes:** the merge cases as tested; Embedder timeout behaviour.

---

## API4. Image and hardening

**Status:** planned · **Depends on:** API3

**Scope**
- Container image `affogato/serving-api:dev` (distroless, non-root).
- Security tests from test-plan §4 that apply to the API (path traversal in keys, header
  injection, oversize query, redirect host pinned to the public endpoint).
- Request metrics on `/metrics`.

**Verify**
```sh
(cd services/serving-api && go test ./...)
docker build -t affogato/serving-api:dev services/serving-api
```

**Report includes:** image size; the security cases covered and any left for later.

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
