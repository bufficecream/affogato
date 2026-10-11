# FramePulse test suite: how to run it

This directory holds the acceptance (BDD) suite for FramePulse and its supporting tooling:

| Path | What it is |
|---|---|
| `acceptance/` | Go module: godog runner (`main_test.go`), step definitions (`steps/`), helpers with unit tests (`internal/`), feature files (`features/`, copied verbatim from `framepulse_spec.md` section 4) |
| `fixtures/` | Media fixture generator (`make_fixtures.sh`), verifier (`verify_fixtures.sh`), speech clip drop folder (`speech/`) |
| `kind/` | kind cluster config, the in-cluster runner Job + RBAC, host port-forward script |
| `go-docker.sh` | Runs `go` in the `golang:1.26-bookworm` container |
| `ffmpeg-docker.sh`, `ffprobe-docker.sh` | Run `ffmpeg` / `ffprobe` (or a script needing them) in a pinned ffmpeg container |
| `Makefile` | Entry points, see below |

Test strategy, traceability and open spec gaps: `docs/test-plan.md`.

Status (2026-10-11, spec v3.1): no FramePulse service exists yet, so all 25 scenarios are
**pending** (every step is defined and returns "pending" with a TODO). The commands
marked "verified" below were run on macOS (arm64) with Docker only.

## 1. Prerequisites

Required: **Docker** (Docker Desktop or any engine; the commands use `docker run` and
`docker build`). That is enough for everything except the cluster run.

Optional:
- Go 1.26: without it, pass `GO="$PWD/tests/go-docker.sh"` (section 2).
- ffmpeg/ffprobe with libx264: without them, fixtures are generated in a container
  automatically (section 4).
- **kind** and **kubectl**: only for running the scenarios against a cluster (section 6).
  Also needed: Helm and the FramePulse chart (`deploy/helm/framepulse`), which does not exist yet.

Run all commands from the repository root unless noted.

## 2. Go in a container (`go-docker.sh`)

`tests/go-docker.sh` behaves like `go`. It mounts the repository at `/repo` and keeps
the module and build caches in the Docker volumes `fp-gomod` and `fp-gocache`. It passes
through `NO_COLOR`, `GODOG_TAGS`, `GODOG_STRICT` and `GODOG_FORMAT`. Override the Go version
with `GO_VERSION=1.26`.

```sh
make -C tests unit vet acceptance-dry GO="$PWD/tests/go-docker.sh"   # verified
cd tests/acceptance && ../go-docker.sh test ./internal/...            # verified
```

The first run downloads the image and modules. Later runs take a few seconds.

## 3. Makefile targets

`make -C tests <target>`; knobs are passed as `make VAR=value`.

| Target | What it does | Needs | Verified |
|---|---|---|---|
| `unit` | Unit tests for the helpers (`internal/...`) and step coverage (`steps/`): every feature step matches exactly one definition, every definition is used and returns pending, the feature files equal the spec's Gherkin blocks. An in-process nats-server is used, no external infrastructure | Go | yes |
| `vet` | `go vet ./...` on the acceptance module | Go | yes |
| `acceptance-dry` | Runs godog over **all** scenarios, including `@load`, with no cluster. Undefined or ambiguous steps fail the run; pending steps are OK unless `GODOG_STRICT=true` | Go | yes |
| `fixtures` | Generates the media fixtures into `tests/fixtures/media/` (git-ignored) and verifies them (section 4) | Docker or host ffmpeg | yes |
| `fixtures-verify` | Re-runs only the fixture verifier | Docker or host ffmpeg | yes |
| `manifests-lint` | Validates `tests/kind/*.yaml` with kubeconform (`ghcr.io/yannh/kubeconform:v0.6.7`, Kubernetes 1.33 schemas). The Job's placeholders are substituted first. The kind `Cluster` config has no schema and is reported as skipped. Downloads schemas, so it needs network but no cluster | Docker | yes |
| `image` | Builds the runner image `framepulse/acceptance:dev` from `tests/acceptance/Dockerfile` (build context: repository root). The build runs vet and the unit tests, compiles the test binary, generates and verifies the fixtures with the pinned ffmpeg image, and adds kubectl v1.36.1 (checksum-verified). The image runs as uid 65532 | Docker | yes |
| `kind-up` | Creates kind cluster `framepulse` (`KIND_NODE_IMAGE` pins the node image) and namespace `framepulse` | kind, kubectl | **no** |
| `kind-down` | Deletes the cluster | kind | **no** |
| `kind-load` | `image`, then `kind load docker-image` | kind | **no** |
| `acceptance` | `kind-load`, then runs the in-cluster Job (`kind/run-job.sh`) with tags `~@load`, streams logs, exits with the Job result | kind cluster + chart | **no** |
| `load` | Same, only `@load` scenarios (Job `fp-acceptance-load`) | kind cluster + chart | **no** |
| `acceptance-host` | Runs godog from the host against port-forwarded services (`~@load`) | cluster + `kind/port-forward.sh` + Go | **no** |

Makefile knobs: `GO` (default `go`), `FFMPEG`, `FFPROBE`, `FIXTURES_DOCKER` (0/1),
`FIXTURES_DIR` (default `tests/fixtures/media`), `IMAGE` (default
`framepulse/acceptance:dev`), `DOCKER`, `KIND`, `KUBECTL`, `KIND_CLUSTER`,
`KIND_NODE_IMAGE`, `NAMESPACE` (default `framepulse`), `GODOG_STRICT` (default `false`),
`KUBECONFORM_IMAGE`, `KUBE_SCHEMA_VERSION`.

Expected `acceptance-dry` result today (verified):

```
25 scenarios (25 pending)
150 steps (25 pending, 125 skipped)
    main_test.go:137: pending steps: 25 (strict=false)
--- PASS: TestFeatures
```

godog skips the remaining steps of a scenario after the first pending one. `make unit`
calls every step directly, so wrong step text is still caught (`steps/coverage_test.go`).

## 4. Media fixtures

```sh
make -C tests fixtures             # generate + verify, verified: "103/103 checks passed"
make -C tests fixtures-verify      # verify only
```

How ffmpeg is chosen (`tests/fixtures/make_fixtures.sh`):
1. `FFMPEG` set: use it (`FFPROBE` defaults to the matching `ffprobe`; for
   `FFMPEG=$PWD/tests/ffmpeg-docker.sh` it is `ffprobe-docker.sh`). This runs one container
   per ffmpeg call, which is slower (about 25 s instead of about 6 s).
2. Otherwise a host `ffmpeg` + `ffprobe`, if both exist.
3. Otherwise, or with `FIXTURES_DOCKER=1`: the whole script re-runs once inside
   `linuxserver/ffmpeg:version-7.1.1-cli` (pinned by digest in `tests/ffmpeg-docker.sh`;
   override with `FFMPEG_IMAGE`).

The wrapper mounts the repository and the current directory at their host paths and runs
as your uid, so output files belong to you. Absolute paths outside the repo (for example
`SPEECH_SRC=/tmp/x.aiff` or `FIXTURES_DIR=/tmp/fx`) are mounted automatically.

Ad-hoc use:

```sh
tests/ffmpeg-docker.sh -version                                       # verified
tests/ffprobe-docker.sh -v error -show_streams tests/fixtures/media/chunk_no_audio.ts
tests/ffmpeg-docker.sh bash -c 'ffprobe -version | head -1'
```

With the pinned image the output is bit-reproducible: two runs produce identical
`SHA256SUMS`, and the runner image contains the same bytes (verified). The fixture files,
what they are for, and the known ffprobe behaviours are described in
`tests/fixtures/README.md`.

### Speech clip (needed for Feature 3 "speech" and Feature 6)

ffmpeg cannot synthesise speech, so you have to provide a recording. Put an English
recording about data pipelines at
`tests/fixtures/speech/data_pipelines_en.{wav,m4a,aiff,mp3}`, or set
`SPEECH_SRC=/path/to/file`. Audio files in that folder are git-ignored. Requirements: at
least 16 s, and the words "data pipeline" and "architecture" in every 2 s window (each
chunk is transcribed on its own). See `tests/fixtures/README.md` for a script to read.
On macOS a TTS clip works for the generator:

```sh
T="Data pipeline architecture. A data pipeline moves events from ingestion to storage. This data pipeline architecture uses streams and workers."
say -v Samantha -r 170 -o tests/fixtures/speech/data_pipelines_en.aiff "$T $T $T $T"
make -C tests fixtures      # now also builds speech/ and chunk_speech_en.ts
```

Verified with a clip outside the repo (`SPEECH_SRC=/tmp/…/clip.aiff`): 18 speech chunks,
the first 8 audible, 115/115 checks. The verifier does **not** check what is said. Before
relying on a clip, confirm with faster-whisper `small` that every chunk 0..7 transcribes
with the expected words and language `en`. Without a clip the speech checks print `SKIP`;
`FIXTURES_STRICT_SPEECH=1` turns that into a failure (use it in CI once a clip is
committed or provided).

The runner image copies `tests/fixtures/` (including `speech/`) into its fixture stage, so a
clip present at build time ends up in the image. Generated media is kept out of the build
context by `tests/acceptance/Dockerfile.dockerignore`.

## 5. Selecting features and scenarios; pending and strict mode

Tags: every feature file has a feature tag: `@hot-path`, `@finalization`, `@cold-path`,
`@failures`, `@idempotency`, `@search`, `@retention`. Latency scenarios are also tagged
`@load`. Without explicit tags the runner uses `~@load` (the default for PR runs).
`acceptance-dry` passes an empty tag expression, so it runs everything.

Options are flags (`-godog.*`) with environment fallbacks:

| Env | Flag | Default | Meaning |
|---|---|---|---|
| `GODOG_TAGS` | `-godog.tags` | `~@load` | tag expression (`@a`, `~@a`, `,` = or, `&&` = and) |
| `GODOG_FORMAT` | `-godog.format` | `pretty` | `pretty`, `progress`, `junit:/path.xml`, comma-separated |
| `GODOG_PATHS` | `-godog.paths` | `features` | feature files or dirs; `file.feature:LINE` selects one scenario |
| `GODOG_STRICT` | `-godog.strict` | `false` | fail on pending steps |
| `GODOG_RANDOM` | `-godog.random` | `0` | random scenario order seed |
| `NO_COLOR` | | | disable ANSI colours |

Flags win over env. Concurrency is fixed at 1, because scenarios share cluster state.
Examples (all verified with `go-docker.sh`, run from `tests/acceptance`):

```sh
cd tests/acceptance
# one feature
NO_COLOR=1 ../go-docker.sh test -count=1 -run '^TestFeatures$' . -v -args -godog.tags=@finalization -godog.format=progress
# one scenario, by line number of its "Scenario:" line
NO_COLOR=1 ../go-docker.sh test -count=1 -run '^TestFeatures$' . -v -args -godog.paths=features/02_finalization.feature:35
# tag expression via env
GODOG_TAGS='@failures && ~@load' ../go-docker.sh test -count=1 -run '^TestFeatures$' .
```

Strict mode, from the repository root (expected to FAIL today with 25 pending, verified):

```sh
make -C tests acceptance-dry GO="$PWD/tests/go-docker.sh" GODOG_STRICT=true
```

Pending means that a step is defined but not implemented yet. Implemented steps return
`godog.ErrPending` until the services exist, for example when the protobuf codec is
missing (`payload.ErrNotImplemented`). Non-strict runs pass with pending steps; strict
runs fail. Undefined or ambiguous steps always fail, in both modes. Once the services
and `gen/go` exist, CI should run with `GODOG_STRICT=true`.

The feature files must stay byte-identical to the spec. `TestFeaturesMatchSpec` (part of
`make unit`) fails if they drift. To re-sync, change the spec and copy the Gherkin blocks
back, keeping the two comment lines and the tag line at the top of each file.

## 6. Running against kind (UNVERIFIED: kind is not installed on the dev machine yet)

None of these commands has been run. The FramePulse Helm chart does not exist yet. The
service names, secret names and `FP_WORKLOAD_*` defaults in `kind/acceptance-job.yaml`,
`kind/port-forward.sh` and `internal/config` are placeholders until it does.

Intended flow:

```sh
make -C tests kind-up                       # cluster "framepulse", namespace "framepulse"
helm install framepulse deploy/helm/framepulse -n framepulse \
  -f deploy/helm/framepulse/values-kind.yaml --wait
make -C tests acceptance                    # build + kind load image, run Job (~@load), ~35 min
make -C tests load                          # @load scenarios only, separate Job
make -C tests kind-down
```

`values-kind.yaml` (owned by the developer's chart) must set:
- Stream Worker `FINALIZE_IDLE_TIMEOUT=10s` and `FINALIZE_SWEEP_INTERVAL=2s` (spec 2.2/A15;
  production keeps 5m/30s). The Feature 2 idle scenario reads the env from the
  deployment and fails if it is not `10s`.
- `testFaults.enabled: true`, which renders `FP_TEST_FAULTS=1` into the Stream Worker and
  AI Worker (spec 2.5). The Feature 4 crash loop and Feature 5 redelivery scenarios publish
  events with the `Fp-Test-Fault` header and check that the env is `1`. Production values
  must never set it.
- `MINIO_PUBLIC_ENDPOINT` equal to the Job's `FP_MINIO_PUBLIC_ENDPOINT`.

Job budget: `activeDeadlineSeconds: 3300`, `-test.timeout 50m`. The long scenarios are the
AI crash loop (about 6 min), AI retry to DLQ (about 2 min of backoff) and redrive.

Host run (also unverified): start `tests/kind/port-forward.sh` in one terminal, export
`FP_MINIO_ACCESS_KEY`/`FP_MINIO_SECRET_KEY` from the MinIO secret, then
`make -C tests acceptance-host`. It needs a local Go, or
`GO="$PWD/tests/go-docker.sh"` with Docker networking that can reach the host's
forwarded ports (on Docker Desktop: set `FP_*_URL` to `host.docker.internal`; untested).

## 7. Runner configuration (`FP_*`)

The defaults target the host + port-forward setup. The Job overrides them with in-cluster
service names.

| Variable | Default | Purpose |
|---|---|---|
| `FP_NATS_URL` | `nats://127.0.0.1:4222` | NATS |
| `FP_NATS_CREDS` | unset | path to a `.creds` file (secret) |
| `FP_MINIO_ENDPOINT` | `127.0.0.1:9000` | MinIO host:port used by the runner |
| `FP_MINIO_PUBLIC_ENDPOINT` | `http://127.0.0.1:9000` | host that presigned URLs must point at |
| `FP_MINIO_ACCESS_KEY` / `FP_MINIO_SECRET_KEY` | unset | MinIO credentials (secret, redacted in logs) |
| `FP_MINIO_USE_TLS` / `FP_MINIO_REGION` | `false` / `us-east-1` | |
| `FP_QDRANT_URL` / `FP_QDRANT_API_KEY` | `http://127.0.0.1:6333` / unset | Qdrant REST |
| `FP_API_BASE_URL` | `http://127.0.0.1:8080` | Serving API |
| `FP_KUBE_NAMESPACE` / `FP_KUBE_CONTEXT` / `FP_KUBECTL` | `framepulse` / current / `kubectl` | fault injection and inspection |
| `FP_WORKLOAD_STREAM_WORKER` | `deployment/framepulse-stream-worker` | also read for `FINALIZE_IDLE_TIMEOUT` / `FP_TEST_FAULTS` |
| `FP_WORKLOAD_AI_WORKER` | `deployment/framepulse-ai-worker` | crash-loop restarts, health check |
| `FP_WORKLOAD_EMBEDDER` / `FP_WORKLOAD_QDRANT` / `FP_WORKLOAD_API` | `deployment/framepulse-embedder` / `statefulset/framepulse-qdrant` / `deployment/framepulse-serving-api` | |
| `FP_PRODUCER_BIN` / `FP_FPCTL_BIN` / `FP_FFPROBE_BIN` | `fp-producer` / `fpctl` / `ffprobe` | binaries in the runner (Producer and fpctl not in the image yet: TODO stages in the Dockerfile) |
| `FP_FIXTURES_DIR` | `../fixtures/media` (image: `/fixtures`) | fixtures |
| `FP_STREAM_WORKER_METRICS_PORT` | `9090` | `/metrics` scraping |
| `FP_EVENT_TIMEOUT` | `30s` | hot-path waits |
| `FP_COLD_PATH_TIMEOUT` | `2m` | STT + embedding waits |
| `FP_FINALIZE_TIMEOUT` | `30s` | idle finalization waits (kind 10 s + 2 s sweep). Set `7m` against production defaults |
| `FP_RETRY_TIMEOUT` | `8m` | NAK/backoff/DLQ waits, incl. the ~6 min AI crash loop |
| `FP_ROLLOUT_TIMEOUT` | `3m` | kubectl scale/rollout waits |
| `FP_HTTP_TIMEOUT` / `FP_MAX_RESPONSE_BYTES` | `10s` / 8 MiB | per request / body cap |

The Qdrant point-ID namespace is no longer configurable. It is the fixed
`c1907ffe-0c06-4077-8866-08fb8d658b77` (spec A18), checked against the B3 golden IDs in
`make unit`.

## 8. Troubleshooting

- **`ffmpeg lacks libx264` although it has it:** fixed. An older script used
  `ffmpeg -encoders | grep -q` under `pipefail`, which fails with SIGPIPE (141).
- **`expected 6 chunks from sample_12s.mp4, got 5`:** the segment muxer missed the first
  2 s boundary. The script uses `-segment_time_delta 0.1`; keep it if you change the cut
  command.
- **Fixture verifier FAIL lines:** each line names the file and the property. Regenerate
  with `make -C tests fixtures`. A failure on `chunk_truncated.ts "ffprobe now fails"`
  means ffmpeg changed behaviour: update the docs, not just the check.
- **`docker: permission denied` / files owned by root:** the wrappers run as your uid. On
  Linux your user must be able to run Docker.
- **Paths not found inside the ffmpeg container:** only the repo, the current directory,
  absolute argument paths and `FFMPEG_DOCKER_MOUNTS` (space-separated dirs) are mounted.
- **`make image` very slow at `apt-get`:** only the kubectl stage uses apt now. A slow
  Debian mirror shows up there; the layer is cached afterwards.
- **`TestFeaturesMatchSpec` fails:** a feature file differs from `framepulse_spec.md`
  section 4. Re-copy the block; never edit the scenarios only in `features/`.
- **`undefined step` / `ambiguous step`:** a spec step changed; adjust the regex in
  `acceptance/steps/*.go` (one definition per step text).
- **Idle-finalization scenarios time out (cluster):** check that `values-kind.yaml` sets
  `FINALIZE_IDLE_TIMEOUT=10s`. Against production values, set `FP_FINALIZE_TIMEOUT=7m`.
- **Crash-loop / redelivery scenarios fail at the start (cluster):** the worker
  deployments do not have `FP_TEST_FAULTS=1` (`testFaults.enabled: true` missing).
- **Colours in logs:** set `NO_COLOR=1`.
