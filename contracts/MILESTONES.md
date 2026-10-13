# Contracts milestones (C1–C5)

Covers `proto/`, `gen/`, `contracts/` and `libs/`. Source of truth:
[`docs/architecture-review.md`](../docs/architecture-review.md) Part B (B1 NATS, B2 object
storage, B3 Qdrant, B4 layout and CI) and [`docs/test-plan.md`](../docs/test-plan.md) §1.1.
Index and report format: [`docs/implementation-plan.md`](../docs/implementation-plan.md).

Every other module depends on C4, so this track goes first. C3 does not depend on C1 and can
be done in parallel.

Host tools: `buf` (`brew install bufbuild/buf/buf`), Go (host or `tests/go-docker.sh`), Rust
(on the host), Python 3.12 (venv under `libs/python`).

---

## C1. Proto module and generated code

**Status:** done 2026-10-13 (branch `feat/c1-proto-codegen`) · **Depends on:** — · **Parallel with:** C3, H0, AI0

**Goal:** the six proto packages compile, pass lint, and the generated code is committed and
builds in all three languages.

**Scope**
- `buf.yaml` (v2, lint STANDARD, breaking FILE) and `buf.gen.yaml` at the repository root;
  module root `proto/`.
- `proto/affogato/{common,events,dlq,state,artifacts,embed}/v1/*.proto`, exactly as B1
  "Proto files" (field numbers, `reserved 2; reserved "key";` in `Segment`, enum zero values
  `*_UNSPECIFIED`).
- `gen/go` (protoc-gen-go, `paths=source_relative`), `gen/rust` (crate `af-proto`, prost),
  `gen/python` (dist `affogato-contracts`, protobuf + `.pyi`). Committed, never hand-edited;
  each has a short README saying so.
- A root `Makefile` target `contracts-gen` (= `buf generate`) and `contracts-check`
  (format, lint, generate + diff), so the CI steps can be run locally.

**Out of scope:** validation logic (C4), golden data (C2).

**Verify**
```sh
buf format --diff --exit-code
buf lint
buf generate && git diff --exit-code gen/
(cd gen/go && go build ./...)
cargo build --manifest-path gen/rust/Cargo.toml
python -I -c "import affogato.events.v1.chunk_events_pb2"   # inside the libs/python venv with gen/python installed
```

**Report includes:** the list of messages and enums per package; any change against B1 (should
be none) and why.

### C1 Proto module and generated code: done 2026-10-13

**Implemented:**
- `buf.yaml` (v2, module `proto`, lint STANDARD, breaking FILE) and `buf.gen.yaml` (remote
  plugins pinned to exact versions) at the repository root.
- `proto/affogato/*/v1/*.proto`, 7 files, 6 packages:

  | Package | File | Messages | Enums |
  |---|---|---|---|
  | `affogato.common.v1` | `common.proto` | `ChunkRef`, `ObjectRef`, `TimeRange`, `MediaInfo`, `EventMeta` | — |
  | `affogato.events.v1` | `chunk_events.proto` | `VideoChunkUploaded` (`reserved "is_last"`), `ChunkHlsReady`, `ChunkTranscriptIndexed` | `IndexOutcome` |
  | `affogato.events.v1` | `video_events.proto` | `VideoIngestCompleted` | — |
  | `affogato.dlq.v1` | `dead_letter.proto` | `Header`, `DeadLetter` | `ErrorClass` |
  | `affogato.state.v1` | `hls_index.proto` | `HlsIndex`, `Segment` (`reserved 2; reserved "key";`) | `FinalizeReason`, `SegmentStatus` |
  | `affogato.artifacts.v1` | `transcript.proto` | `Transcript`, `TranscriptSegment` | — |
  | `affogato.embed.v1` | `embed.proto` | `EmbedRequest`, `EmbedResponse`, `Vector` | `Purpose` |

  Every enum's zero value is `*_UNSPECIFIED`; `go_package` = `github.com/affogato/affogato/gen/go/affogato/<pkg>/v1;<pkg>v1`.
- `gen/go`: module `github.com/affogato/affogato/gen/go` (`go.mod`, `go.sum`), 7 `*.pb.go`.
- `gen/rust`: crate `af-proto` (`Cargo.toml`, committed `Cargo.lock`), `src/lib.rs` plus one
  file per package; `use af_proto::affogato::events::v1::VideoChunkUploaded`.
- `gen/python`: dist `affogato-contracts` (`pyproject.toml`, hatchling), `src/affogato/**/*_pb2.py`
  and `.pyi`; `affogato` is an implicit namespace package.
- Each `gen/*` has a README saying "generated, do not edit, regenerate with `make contracts-gen`".
- Root `Makefile`: `contracts-gen` (removes old generated files, then `buf generate`) and
  `contracts-check` (format, lint, generate, `git diff --exit-code gen/` plus an untracked-file
  check, `go build`/`go vet`, `cargo build --locked`, venv install and import of all 7 Python
  modules). Root `.gitignore` for `/.venv/`, `/gen/rust/target/`, `__pycache__/`.

**Verified** (run 1 in the development worktree; run 2 on the committed branch after
`git clean -xdf` removed the venv and `gen/rust/target/`, with `GO=tests/go-docker.sh`;
shared Go/cargo/pip download caches were kept):
- `buf format --diff --exit-code`: no diff, exit 0 (both runs).
- `buf lint`: clean, exit 0 (both runs).
- `buf generate && git diff --exit-code gen/`: exit 0, 36 committed files reproduced
  byte-identically (both runs).
- `(cd gen/go && go build ./...)`: exit 0 (run 1 host go1.27.2 and `tests/go-docker.sh`
  golang:1.26; run 2 `tests/go-docker.sh`); `go vet ./...` clean.
- `cargo build --manifest-path gen/rust/Cargo.toml`: `Finished dev profile`, no warnings
  (both runs); additionally `cargo clippy` clean and `cargo test` 0 tests / 0 doctests failing.
- `python -I -c "import affogato.events.v1.chunk_events_pb2"`: exit 0 in the venv with
  `affogato-contracts 0.1.0` and `protobuf 7.36.2` (both runs); all 7 modules import.
- `make contracts-check`: `contracts-check: OK` (both runs).
- Deliberate failure: a proto comment edited without regenerating makes `make contracts-check`
  stop at `git diff --exit-code -- gen/` (exit 2).
- Already green: `make -C tests unit acceptance-dry GO="$PWD/tests/go-docker.sh"`: all unit
  packages ok; 25 scenarios (25 pending), 150 steps (25 pending, 125 skipped), unchanged.

**Acceptance:** no scenario changes (contracts only).

**Deviations:**
- None in field numbers, names or types against B1. B1's abbreviated "supporting contracts"
  (`state`, `artifacts`, `embed`) are written out with the listed fields and numbers; the added
  comments only restate the spec (key, writer/reader, finalization rules).
- B1's note "Field 3 is held for `tenant_id`" in `ChunkRef` stays a comment, as in B1; it is
  not a `reserved 3` (that would make a later `tenant_id = 3` a breaking change under FILE).
- Python verify uses a git-ignored venv at the repository root (`PY_VENV`, default `.venv`)
  instead of the `libs/python` venv, which does not exist until C4; C4 can point `PY_VENV` at
  its own venv. The host interpreter is Python 3.14 (`PYTHON`, default `python3`), not 3.12;
  the package requires `>=3.10` (protobuf 7).

**Decisions made:**
- buf CLI: container `bufbuild/buf:1.73.0` (released 2026-09-11) via `BUF ?=` in the
  `Makefile`, run as the host uid/gid; override with `BUF=buf` (a host buf must be 1.73.0, or
  format/generate output may differ). `GO` defaults to host `go` when present, else
  `tests/go-docker.sh`.
- Remote BSR plugins, exact versions, no local plugin installs: `protocolbuffers/go:v1.36.12`
  (`paths=source_relative`, default open struct API), `community/neoeinstein-prost:v0.5.0`
  (`bytes=.`, so `DeadLetter.original_payload` is `bytes::Bytes` like async-nats payloads),
  `community/neoeinstein-prost-crate:v0.5.0` (`no_features`, `include_file=lib.rs`),
  `protocolbuffers/python:v36.2` and `protocolbuffers/pyi:v36.2`.
- Runtime pins that match the plugins: `google.golang.org/protobuf v1.36.12`; `prost` /
  `prost-types` `0.14.1` (resolved to 0.14.4 in `Cargo.lock`); `protobuf>=7.36.2,<8` (the
  generated Python checks the runtime version on import). Build backend `hatchling==1.27.0`.
- `buf.gen.yaml` does not use `clean: true` (it would delete the hand-written `go.mod`,
  `Cargo.toml`, `pyproject.toml`); `make contracts-gen` deletes only generated files first.
- The Python plugin stamps "NO CHECKED-IN PROTOBUF GENCODE" in each `_pb2.py`; that is
  upstream's default advice, and the code is committed anyway as B4 and this milestone require.

**Open issues / next:**
- `buf breaking --against '.git#branch=main,subdir=proto'` is not in `contracts-check`, because
  `main` has no `proto/` yet; C5 (CI) adds it once C1 is merged.
- No `py.typed` marker yet, so mypy treats the installed `affogato-contracts` as untyped and
  ignores its `.pyi` stubs. `affogato` is a namespace package, so the marker belongs in the
  `v1` subpackages; C4 adds it when it wires the package into `libs/python`.
- C2 (golden data, `keys.json`) builds on these messages next.

---

## C2. Golden test data and `keys.json`

**Status:** planned · **Depends on:** C1

**Goal:** one source of truth for every derived name and one encoded example of every message,
so the three languages can be checked against the same data.

**Scope**
- `contracts/testdata/*.txtpb` for every message in C1 (at least one realistic and one
  minimal instance each), converted with `buf convert` to `*.binpb`; both committed. A
  script `contracts/testdata/regen.sh` rebuilds the `.binpb` files.
- `contracts/testdata/keys.json` with `point_id_namespace` and the cases from test-plan §1.1:
  - `chunk_seq` 0, 5, 42, 7999, 8000, 999999 (and 1000000 as a rejection case);
  - every subject, the four per-video filter subjects, the five `Nats-Msg-Id` forms plus the
    redrive form, every object key, the `HLS_INDEX` key, API paths, `timestamp` strings;
  - point IDs including the two B3 golden IDs;
  - invalid `video_id`s (uppercase, UUIDv4, braces, trailing newline, `*`, `>`, `.`, `/`);
  - `Af-Test-Fault` values: valid and each rejection class.
- A JSON Schema for `keys.json` (`contracts/testdata/keys.schema.json`), so a malformed edit
  fails early.

**Verify**
```sh
contracts/testdata/regen.sh && git diff --exit-code contracts/testdata/
buf convert proto --type affogato.state.v1.HlsIndex --from contracts/testdata/hls_index_full.binpb --to -#format=json
python -I -m jsonschema -i contracts/testdata/keys.json contracts/testdata/keys.schema.json
```

**Report includes:** the number of cases per category in `keys.json`; the `.binpb` file list.

---

## C3. NATS and Qdrant configuration

**Status:** done 2026-10-13 · **Depends on:** — · **Parallel with:** C1, C2

**Goal:** the stream, consumer, KV and collection definitions exist as files that the Helm
chart (H1) applies unchanged, and they are proven valid against real servers.

**Scope**
- `contracts/nats/streams/{VIDEO_EVENTS,VIDEO_DLQ,AF_ADVISORIES}.json` (B1 table: subjects,
  limits retention, max_age 7d, max_bytes, max_msg_size, discard, duplicate_window,
  deny_delete/purge, allow_direct). Replicas are 3 in the file; the chart overrides to 1 on
  kind.
- `contracts/nats/consumers/{hls-stream-worker,ai-indexer}.json` (filters, ack_wait 15s/60s,
  max_deliver 10, max_ack_pending 512/32).
- `contracts/nats/kv/HLS_INDEX.json` (history 1, max_value_size 1 MiB).
- `contracts/qdrant/transcripts_me5s384_v1.json` (384-d, Cosine, payload indexes from B3) and
  the alias `transcripts`.
- `contracts/check-config.sh`: starts a throwaway `nats:2.15.0-alpine -js` and
  `qdrant/qdrant:v1.19.2` container (both already preloaded), applies every file with the
  `nats` CLI (from `natsio/nats-box:0.20.0`) and the Qdrant REST API, reads them back and
  compares, then removes the containers.

**Verify**
```sh
contracts/check-config.sh          # expected: every stream/consumer/KV/collection created and read back equal
```
Plus a check that the retention values match Feature 7 (`07_retention.feature`): streams 7 d,
and the DLQ age shorter than the planned `af-raw` expiry (10 d, set in H1).

**Report includes:** each file and the server's read-back result; any setting the server
normalised or rejected.

### C3 NATS and Qdrant configuration: done 2026-10-13
**Implemented:**
- `contracts/nats/streams/{VIDEO_EVENTS,VIDEO_DLQ,AF_ADVISORIES}.json`: server `StreamConfig`
  JSON (`nats stream add --config`), values from the B1 table, durations in nanoseconds,
  `num_replicas` 3.
- `contracts/nats/consumers/{hls-stream-worker,ai-indexer}.json`: server `ConsumerConfig`; the
  stream is named in `metadata."affogato.stream"` because `ConsumerConfig` has no stream field.
- `contracts/nats/kv/HLS_INDEX.json`: `KeyValueConfig` (nats.go `jetstream`), history 1,
  `max_value_size` 1 MiB, file, replicas 3.
- `contracts/qdrant/transcripts_me5s384_v1.json` (`collection_name`, `collection` = create
  body, `payload_indexes` = index bodies) and `contracts/qdrant/aliases.json` (the
  `POST /collections/aliases` body: `transcripts` → `transcripts_me5s384_v1`).
- `contracts/nats/README.md`, `contracts/qdrant/README.md`: file shapes, idempotent apply
  commands, server-JSON → NACK CRD field mapping (for H1).
- `contracts/check-config.sh`: throwaway `nats -js`, Qdrant and nats-box containers on a
  private network (random suffix, no host ports, image refs `tag@digest` from
  `tests/kind/images.txt`). It applies every file twice, compares every field the file sets
  with the server read-back, checks Feature 7 retention and that `AF_ADVISORIES` captures a
  real MAX_DELIVERIES advisory, then removes everything (EXIT trap).

**Verified:**
- `contracts/check-config.sh` run 1: `SUMMARY: 19 passed, 0 failed`, exit 0, ~10 s; run 2
  right after: identical output apart from the random suffix, exit 0. After each run,
  `docker ps -a --filter name=af-c3check` and `docker network ls --filter name=af-c3check`
  list nothing.
- Read-back per file (applied with replicas 1, the rest unchanged):

  | File | Server read-back |
  |---|---|
  | `streams/VIDEO_EVENTS.json` | 15 fields equal (s2, 7d, 10 GiB, 64 KiB, old, 10m, deny_delete, allow_direct) |
  | `streams/VIDEO_DLQ.json` | 14 fields equal (7d, 1 GiB, 256 KiB, **new**, 10m, deny_delete, allow_direct) |
  | `streams/AF_ADVISORIES.json` | 11 fields equal (7d, 100 MiB, max_msg_size -1, old, 2m); probe advisory `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.VIDEO_EVENTS.c3-probe` captured |
  | `consumers/hls-stream-worker.json` | 10 fields equal (2 filters, all, explicit, 15s, 10, 512) |
  | `consumers/ai-indexer.json` | 10 fields equal (1 filter, all, explicit, 60s, 10, 32) |
  | `kv/HLS_INDEX.json` | 6 fields equal via stream `KV_HLS_INDEX` (history = `max_msgs_per_subject` 1, `max_value_size` = `max_msg_size` 1048576, file) |
  | `qdrant/transcripts_me5s384_v1.json` | 384 / Cosine, `on_disk_payload` true, metadata, 4 payload indexes equal; HNSW defaults m 16, ef_construct 100, full_scan_threshold 10000 |
  | `qdrant/aliases.json` | `transcripts -> transcripts_me5s384_v1`; `GET /collections/transcripts` returns the 384-d Cosine config and model |

- Feature 7: `VIDEO_EVENTS`, `VIDEO_DLQ`, `AF_ADVISORIES` `max_age` 7d in file and on server;
  `VIDEO_DLQ max_age 7d < planned af-raw expiry 10d (H1)`.
- Mutation run on a scratch copy (DLQ `max_age` 11d, `compression: "none"`, unknown consumer
  stream, KV replicas 1, Qdrant `"cosine"`): `SUMMARY: 11 passed, 7 failed`, exit 1, every
  mutation reported, containers removed.
- `shellcheck` (`koalaman/shellcheck:v0.11.0`, default rules, `-x`): clean, exit 0.
- `make -C tests unit acceptance-dry GO="$PWD/tests/go-docker.sh"`: 13 packages ok;
  `25 scenarios (25 pending)`, `--- PASS: TestFeatures`.

**Acceptance:** none newly green. 7.1 also needs the MinIO lifecycle half (H1, ACC1); its NATS
half is pre-checked here.

**Deviations:**
- The script applies replicas 1. A single-node server rejects R3 (`replicas > 1 not supported
  in non-clustered mode`, 10074), the same reason `values-kind.yaml` overrides.
- Consumers leave `num_replicas` unset (0 = inherit the stream) instead of 3, so they follow the
  stream's replica override on kind without a second override.
- `AF_ADVISORIES`: B1 "default" `max_msg_size` written as `-1` (the server default); the "—"
  fields (`deny_delete`, `deny_purge`, `allow_direct`) are omitted (server default false).
- Additions beyond the scope list: the two READMEs, the advisory probe, and
  `collection.metadata.embedding_model` so the alias target's model is visible to the workers'
  startup check (B3).
- The status table in `docs/implementation-plan.md` is not updated (outside this task's file
  scope); C3 should be set to done there.

**Decisions made:**
- Normalised by the server: `"compression": "none"` reads back as absent, so `VIDEO_DLQ` and
  `AF_ADVISORIES` omit `compression` (B1 asks S2 only for `VIDEO_EVENTS`). A one-element
  `filter_subjects` stays `filter_subjects` (not folded into `filter_subject`). The server adds
  `_nats.*` metadata, `max_waiting` 512, `name` = durable; the KV backing stream gets discard
  new, `allow_rollup_hdrs`, `deny_delete`, `allow_direct`, duplicate window 2m on its own.
- Rejected by the server: R3 on a single node (above); Qdrant distance `"cosine"` (lowercase,
  HTTP 400), seen in the mutation run.
- Idempotency: `nats stream/consumer/kv add` with an identical config succeeds (a different
  config fails with 10058, so drift is not silently applied). Qdrant `PUT /collections/{name}`
  again is 409, so apply means create-if-missing; payload index `PUT` and `create_alias` are
  repeatable, and `create_alias` re-points an existing alias (atomic model swap).
- Qdrant `replication_factor`/`shard_number` left unset (B3 is silent; read back 1/1). Integer
  payload indexes use the default (lookup + range).
- CLI JSON vs NACK CRD (`jetstream.nats.io/v1beta2`, checked against NACK `deploy/crds.yml`):
  camelCase everywhere; `num_replicas` → `replicas`; durations ns integers → Go duration
  strings (`maxAge: 168h`, `duplicateWindow: 10m`, `ackWait: 15s`); consumer stream →
  `streamName`; NACK stream `storage` defaults to **memory** (server: file), so the chart must
  always render it. Full table in `contracts/nats/README.md`.

**Open issues / next:**
- H1: render NACK `Stream`/`Consumer`/`KeyValue` from these files with the mapping above
  (replicas 1 on kind), and an init Job applying `contracts/qdrant/` as described in its
  README. Set the `af-raw` expiry to 10 d to match `AF_RAW_EXPIRY_DAYS` in the script.
- C5: run `contracts/check-config.sh` as a job step (needs Docker; pulls the three pinned
  images on a clean runner).
- H3: Qdrant `replication_factor` for production, if Qdrant runs clustered.
- Not exercised: cleanup after Ctrl-C (INT trap path); the exit-1 path was exercised.

---

## C4. Contract libraries (Go, Rust, Python)

**Status:** planned · **Depends on:** C2 · **Unblocks:** every service track

**Goal:** each language builds subjects, filters, Msg-Ids, object keys, KV keys and point IDs
the same way and validates IDs and events the same way, proven by `keys.json`.

**Scope**
- `libs/go/afcontracts`, `libs/rust/af-contracts`, `libs/python/af_contracts`, each with:
  - builders for everything listed in C2; key builders reject seq > 999,999;
  - validators: `video_id` (lowercase hyphenated UUIDv7 only), `chunk_seq` < 8000,
    `chunk_count` ≤ 8000, `Af-Test-Fault` grammar (B1 "Test-only fault hooks");
  - the point-ID namespace constant;
  - a decode test for every `contracts/testdata/*.binpb`, including tolerance of unknown
    fields and unknown enum values (evolution rules).
- Tests in each language load `contracts/testdata/keys.json` directly (no copies).
- `tests/acceptance/internal/contracts` is tested against `keys.json` too (the TODO in
  `contracts.go`), so the acceptance harness and the services cannot drift.

**Verify**
```sh
(cd libs/go/afcontracts && go test ./...)
cargo test --manifest-path libs/rust/af-contracts/Cargo.toml
(cd libs/python && python -m pytest -q)
make -C tests unit GO="$PWD/tests/go-docker.sh"
```

**Report includes:** pass counts per language; the number of `keys.json` cases each language
checks (they must be equal).

---

## C5. Contracts CI

**Status:** planned · **Depends on:** C4

**Goal:** a pull request that changes a contract cannot merge unless it is formatted, linted,
not breaking, regenerated, and still matches `keys.json` in all three languages.

**Scope**
- `.github/workflows/contracts.yml` (`bufbuild/buf-action`), B4 CI steps 1–5:
  `buf format --diff --exit-code`, `buf lint`,
  `buf breaking --against '.git#branch=main,subdir=proto'`,
  `buf generate && git diff --exit-code gen/`, and the C4 tests in all three languages.
- `contracts/check-config.sh` (C3) as a job step.

**Verify**
- `make contracts-check` passes locally.
- A throwaway branch that renames a field fails `buf breaking`; a branch that edits `gen/` by
  hand fails the regenerate step. Both results are quoted in the report, then the branches are
  deleted.
- The workflow passes on GitHub (`gh run list --workflow contracts.yml`).

**Report includes:** the workflow run link and the two deliberate-failure results.

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
