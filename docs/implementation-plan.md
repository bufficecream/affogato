# Implementation plan and milestones

Status: **planned, nothing implemented** (2026-10-12). This file is the index. The detailed
milestones live next to the code they belong to, so each module's plan stays with that module:

| Module | Milestone file | IDs | Language |
|---|---|---|---|
| Contracts (`proto/`, `gen/`, `contracts/`, `libs/`) | [`contracts/MILESTONES.md`](../contracts/MILESTONES.md) | C1–C5 | buf, Go, Rust, Python |
| Producer CLI (+ `afctl`) | [`services/producer/MILESTONES.md`](../services/producer/MILESTONES.md) | P1–P4 | Go |
| Stream Worker | [`services/stream-worker/MILESTONES.md`](../services/stream-worker/MILESTONES.md) | SW1–SW5 | Rust |
| Serving API | [`services/serving-api/MILESTONES.md`](../services/serving-api/MILESTONES.md) | API1–API4 | Go (Gin) |
| AI Worker + Embedder | [`services/ai-worker/MILESTONES.md`](../services/ai-worker/MILESTONES.md) | AI0–AI4 | Python |
| Helm chart | [`deploy/helm/affogato/MILESTONES.md`](../deploy/helm/affogato/MILESTONES.md) | H0–H3 | Helm |
| Acceptance (turning scenarios green) | [`tests/acceptance/MILESTONES.md`](../tests/acceptance/MILESTONES.md) | ACC1–ACC4 | Go (godog) |

Behaviour comes from [`affogato_spec.md`](../affogato_spec.md), data contracts from
[`architecture-review.md`](architecture-review.md) Part B, and required tests from
[`test-plan.md`](test-plan.md). A milestone file never restates those; it points at the
section and says what "done" means.

## Decisions needed before or during the work

| ID | Decision | Blocks | Owner |
|---|---|---|---|
| D1 | **Object store.** MinIO no longer publishes images (Docker Hub, Quay; checked 2026-10-11). Build MinIO from source, use a community image, or switch to another S3-compatible store (SeaweedFS, Garage, RustFS) | H0 → H1, all kind runs | you |
| D2 | **H.264 without audio** (test-plan G9): accept as video-only (cold path reports NO_AUDIO) or reject in the Producer | P1 | you |
| D3 | **Embedding library** for `multilingual-e5-small`: fastembed custom ONNX, onnxruntime + tokenizers, or sentence-transformers. Decided by the AI0 measurement | AI1, API3 | developer, reported to you |
| D4 | **Where `afctl` lives** (redrive CLI): proposed `services/producer/cmd/afctl` (shares the Go contract libs) | P4 | you (default: proposal) |

Other spec gaps (test-plan §3, G7–G15) are resolved inside the milestone that hits them and
listed in that milestone's report.

## Order and parallel tracks

```
            ┌─ C1 proto+codegen ─ C2 golden data ─ C4 contract libs ─ C5 CI ─┐
 now ───────┼─ C3 NATS/Qdrant config ─────────────────────────────────────────┤
            ├─ D1 object store → H0 → H1 infra chart ───────── ACC1 (5.1, 7.1)│
            └─ AI0 spikes (embedding lib, Whisper baking)                     │
                                                                              ▼
 after C4 ──┬─ P1 → P2 → P3 → P4        (Go)          ┐
            ├─ SW1 → SW2 → SW3 → SW4    (Rust)        ├─ H2 service templates ─ ACC2 hot path
            ├─ API1 → API2              (Go)          ┘
            └─ AI1 → AI2 → AI3          (Python) ─ API3 ─ ACC3 cold path + search
                                                                    ▼
                                             SW5 / API4 / AI4 / H3 ─ ACC4 load + strict CI
```

- **Can start now, in parallel:** C1→C2→C4, C3, D1→H0→H1, AI0. None of them needs another.
- **After C4, in parallel:** the four service tracks (P, SW, API1–2, AI1–3). Each is unit-tested
  against in-memory fakes, so no service waits for another until kind integration.
- **Strictly sequential:** ACC2 needs P3 + SW4 + API2 + H2 deployed together; ACC3 needs AI3 +
  API3 + Qdrant; ACC4 (strict CI) needs every scenario green.
- **Early green scenarios:** 5.1 (duplicate publish) and 7.1 (retention) only need NATS, the
  object store and the contract config, so ACC1 can pass before any service exists.
- **One developer:** C1→C2→C3→C4, then P→SW→API1–2→H2→ACC2 (first playable video), then
  AI0–AI3→API3→ACC3, then the last milestones. **Two developers:** split Go (C4 Go lib, P, API)
  and Rust/Python (C4 Rust/Python libs, SW, AI), with H0–H1 taken by whoever is free first.

## Definition of done (every milestone)

1. Everything in the milestone's **Scope** exists; anything moved out is listed under
   **Deviations** in the report, with the reason.
2. Every command under **Verify** passes on this machine, run from the repository root, and
   the report quotes each command's summary output (pass counts, not "it works").
3. Required unit tests from test-plan §1.2 for the covered behaviour exist and pass with no
   infrastructure (fakes only); ffmpeg tests use `tests/fixtures/media`.
4. Already-green things stay green: `make -C tests unit acceptance-dry GO="$PWD/tests/go-docker.sh"`
   and any acceptance scenario marked green in [`tests/acceptance/MILESTONES.md`](../tests/acceptance/MILESTONES.md).
5. Docs are updated where behaviour or commands changed (module README, `tests/README.md`).
6. The milestone's status line and **Implementation log** entry are filled in, and the status
   table below is updated.
7. One commit (or a small series) per milestone, with the milestone ID in the subject, e.g.
   `C1: proto module and generated code`.

## Milestone report (what you get after each milestone)

Each milestone ends with this report, written into the module file's **Implementation log**
and summarised to you:

```markdown
### <ID> <title>: done YYYY-MM-DD   (or: blocked / partial)
**Implemented:** what now exists, 3–8 bullets, with paths.
**Verified:** each Verify command with its result line, e.g. `buf lint`: clean;
  `go test ./libs/go/...`: 41 passed.
**Acceptance:** scenarios newly green, still pending, or newly failing (by number, e.g. 2.4).
**Deviations:** what differs from the spec, contracts or this plan, and why.
**Decisions made:** spec gaps resolved here (G-numbers) and choices someone should know.
**Open issues / next:** what is left and which milestone picks it up.
```

## Status

| ID | Title | Depends on | Status |
|---|---|---|---|
| C1 | Proto module and generated code | — | planned |
| C2 | Golden test data and `keys.json` | C1 | planned |
| C3 | NATS and Qdrant configuration | — | planned |
| C4 | Contract libraries (Go, Rust, Python) | C2 | planned |
| C5 | Contracts CI | C4 | planned |
| P1 | Probe, codec gate and length cap | C4, D2 | planned |
| P2 | Segmenter | P1 | planned |
| P3 | Upload, publish, completion | P2 | planned |
| P4 | Binaries for the runner image (`af-producer`, `afctl`) | P3, D4 | planned |
| SW1 | Consumer loop, DLQ rules, fault hooks | C4 | planned |
| SW2 | Remux, probe, thumbnail, object writes | SW1 | planned |
| SW3 | `HLS_INDEX` updates and `ChunkHlsReady` | SW2 | planned |
| SW4 | Finalization, playlists, idle sweeper | SW3 | planned |
| SW5 | Metrics, image, latency budget | SW4 | planned |
| API1 | Skeleton, validation, presigning | C4 | planned |
| API2 | Playlist state machine and media redirects | API1 | planned |
| API3 | Search | API2, AI1 | planned |
| API4 | Image and hardening | API3 | planned |
| AI0 | Spikes: embedding library, Whisper baking | — | planned |
| AI1 | Embedding module and Embedder service | AI0, C4 | planned |
| AI2 | AI Worker pipeline | AI1 | planned |
| AI3 | Failure handling and fault hooks | AI2 | planned |
| AI4 | Image, startup refusals, latency | AI3 | planned |
| H0 | Object store decision | D1 | planned |
| H1 | Infrastructure chart on kind | H0, C3 | planned |
| H2 | Service templates and `values-kind.yaml` | H1 + service images | planned |
| H3 | Production values and chart CI | H2 | planned |
| ACC1 | Infrastructure scenarios (5.1, 7.1) | H1, C4 | planned |
| ACC2 | Hot path and finalization scenarios | P3, SW4, API2, H2 | planned |
| ACC3 | Cold path, failures, search scenarios | AI3, API3, H2 | planned |
| ACC4 | Load scenarios and strict CI | all | planned |
