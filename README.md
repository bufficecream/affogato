# Affogato

A cloud-native, distributed video processing and semantic search pipeline, built to
explore real-time stream processing, asynchronous AI analysis and vector search with
**Go, Rust and Python** on **Kubernetes**.

Upload a video and Affogato:
- turns every ~2 s chunk into an HLS segment within seconds (**hot path**), so the video
  plays on demand as soon as the upload is finished;
- transcribes the speech and indexes it (**cold path**), so you can search *what was
  said* and jump straight to that moment.

> **Status (2026-10-11): design and test-harness stage.** The spec, data contracts and
> the acceptance test framework exist; the services are not implemented yet. All 25
> acceptance scenarios currently report as *pending*.

---

## Architecture at a glance (C4 level 2)

```
 Uploader -> [Producer CLI · Go] --raw chunks--> [MinIO · object storage]
                   | events                          ^          ^
                   v                                 |          |
           [NATS JetStream · event bus] ------+      |          |
             |              |                 |      |          |
             v hot path     v cold path       |      |          |
   [Stream Worker · Rust] [AI Worker · Python]|      |          |
     remux -> HLS +         speech -> text -> |      |          |
     thumbnails ----------- vectors ----------+------+          |
                              |                                 |
                              v                                 |
                       [Qdrant · vector DB]                     |
                              ^                                 |
                              | search                          |
 Viewer -> [Player / Search UI] -> [Serving API · Go/Gin] ------+
                                         |
                                         v
                              [Embedder · Python]  query text -> vector
```

| Container | Language | Role |
|---|---|---|
| Producer CLI | Go | Cuts an `.mp4` (H.264 + AAC, ≤ 4 h) into ~2 s MPEG-TS chunks, uploads them, publishes events |
| NATS JetStream | — | Event streams `VIDEO_EVENTS`, `VIDEO_DLQ`; KV `HLS_INDEX` (per-video segment state) |
| Stream Worker | Rust | Hot path: remux to HLS, thumbnails, segment index, final VOD playlists |
| AI Worker | Python | Cold path: faster-whisper speech-to-text, multilingual-e5-small embeddings |
| Embedder | Python | Embeds search queries with the same model |
| MinIO | — | Raw chunks, segments, thumbnails, transcripts, playlists |
| Qdrant | — | Transcript vectors for semantic search |
| Serving API | Go (Gin) | Playlists, media redirects (presigned URLs), `/api/v1/search` |

The C1 (system context) and C2 (container) diagrams, including Mermaid versions, are in
[`affogato_spec.md` §3](affogato_spec.md).

## Key decisions (MVP)

- **VOD only.** A video becomes playable once it is finalized (all chunks done, or 5 min
  of inactivity). Live playback is Future Work.
- **English first, multilingual-ready.** Whisper `small` by default (`WHISPER_MODEL`
  selects tiny…large-v3); embeddings with `intfloat/multilingual-e5-small`, so Japanese
  and Chinese can follow without re-indexing.
- **Max video length 4 h / 8,000 chunks**, so each video's index fits one NATS KV entry.
- **At-least-once, idempotent processing.** Deterministic keys and IDs; failures go to a
  dead-letter queue after at most 5 attempts and can be redriven.
- **Kubernetes-only testing.** Acceptance tests run on a local `kind` cluster with the
  same Helm chart as production; unit tests need no infrastructure.

The full list (A1–A20) is in [`affogato_spec.md` §5](affogato_spec.md); open ideas
are in §6 *Future Work*.

## Documentation

| Document | What it covers |
|---|---|
| [`affogato_spec.md`](affogato_spec.md) | **Start here.** Overview, architecture, C4 diagrams, processing rules, BDD acceptance criteria (7 features), decisions, Future Work |
| [`docs/architecture-review.md`](docs/architecture-review.md) | Design review and the normative **data contracts**: protobuf messages, NATS streams/consumers/KV, headers, MinIO keys, Qdrant schema, repo layout, CI |
| [`docs/test-plan.md`](docs/test-plan.md) | Test strategy per layer, scenario traceability matrix, gaps and security tests |
| [`docs/implementation-plan.md`](docs/implementation-plan.md) | **Development milestones:** order, parallel tracks, decisions needed, definition of done, report format; links to each module's `MILESTONES.md` |
| [`tests/README.md`](tests/README.md) | **How to run the tests** |
| [`docs/affogato_spec.v1.md`](docs/affogato_spec.v1.md) | Original spec, kept for reference |

## Repository layout

```
affogato_spec.md        spec (source of truth for behaviour)
docs/                     architecture review + contracts, test plan, v1 spec
tests/
  acceptance/             Go module: godog BDD suite, step definitions, test helpers
  fixtures/               script that generates test media with ffmpeg
  kind/                   kind cluster config, in-cluster test Job + RBAC
  Makefile                unit / dry-run / kind / acceptance / load targets
  go-docker.sh            runs `go` inside the official golang container
.claude/agents/           project agents: architect, qa-engineer, developer
```

Planned (from the contracts doc, not created yet): `proto/` (buf module), `gen/`
(generated Go/Rust/Python code), `contracts/` (NATS/Qdrant config, golden test data),
`libs/` (per-language contract helpers), `services/` (producer, serving-api,
stream-worker, ai-worker) and `deploy/helm/affogato`.

## Getting started

**Prerequisite: Docker.** Go and ffmpeg run in containers for now, so nothing else is
needed on the host. Installing Go (`brew install go`), Rust (`rustup`) and `kind` locally
becomes worthwhile once service development starts.

Run the test suite's checks that need no cluster:

```sh
make -C tests unit acceptance-dry GO="$PWD/tests/go-docker.sh"
```

`unit` runs the test-helper unit tests; `acceptance-dry` runs every BDD scenario and
shows it as pending. Generate and verify the test media (ffmpeg runs in a pinned
container automatically when it isn't installed):

```sh
make -C tests fixtures
``` See [`tests/README.md`](tests/README.md) for fixtures, the `kind`
flow, tags and environment variables.

## Working with the project agents

Three Claude Code agents are defined in `.claude/agents/`:

| Agent | Role |
|---|---|
| `architect` | Reviews design, boundaries, scalability and contracts; read-only |
| `qa-engineer` | Finds bugs, edge cases and security issues; writes tests only |
| `developer` | Implements features and fixes to spec |

## Roadmap

The detailed, verifiable milestones for each step are in
[`docs/implementation-plan.md`](docs/implementation-plan.md).

1. **Contracts:** `proto/` + `buf` lint/breaking, generated code, NATS stream/consumer
   config, golden `keys.json`.
2. **Services:** Producer → Stream Worker → Serving API (hot path, playable VOD) →
   AI Worker + Embedder (search).
3. **Deployment:** Helm chart (`values-kind.yaml`, `values-prod.yaml`), then turn the
   pending acceptance scenarios green.
4. **Future Work** (spec §6): live playback, video deletion, ABR ladder, sliding-window
   embeddings, Japanese/Chinese tuning, auth/tenancy, GPU, index sharding.
