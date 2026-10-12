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

**Status:** planned · **Depends on:** — · **Parallel with:** C3, H0, AI0

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

**Status:** planned · **Depends on:** — · **Parallel with:** C1, C2

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
