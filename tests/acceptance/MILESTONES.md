# Acceptance milestones (ACC1–ACC4)

Turning the 25 BDD scenarios from *pending* to *green* on kind. The step definitions already
exist and return pending (`steps/*.go`); each milestone implements the steps for its
scenarios, runs them in the in-cluster Job, and records the result in the table below.

Source of truth: the feature files (byte-identical to the spec), the traceability matrix in
[`docs/test-plan.md`](../../docs/test-plan.md) §2 (scenario numbers below), and
[`tests/README.md`](../README.md) for commands. Index: [`docs/implementation-plan.md`](../../docs/implementation-plan.md).

Run on kind:
```sh
make -C tests kind-up kind-preload        # once
helm upgrade --install affogato deploy/helm/affogato -n affogato -f deploy/helm/affogato/values-kind.yaml --wait
make -C tests acceptance                  # all non-@load scenarios
```

Narrowed runs: `make acceptance` always passes `~@load`, so call the Job script directly after
`make -C tests kind-load`, one tag expression per run:
`tests/kind/run-job.sh affogato/acceptance:dev '@retention'`.

---

## ACC1. Infrastructure scenarios

**Status:** planned · **Depends on:** H1, C4 · **Scenarios:** 5.1, 7.1

**Scope**
- Implement the steps for 5.1 (duplicate publish deduplicated by the stream) and 7.1
  (retention of streams and buckets).
- Wire `internal/payload` to `gen/go` (removes `payload.ErrNotImplemented`).
- Replace placeholder service and Secret names in `kind/acceptance-job.yaml`,
  `kind/port-forward.sh` and `internal/config` with the chart's real names.

**Verify**
```sh
make -C tests unit GO="$PWD/tests/go-docker.sh"
make -C tests kind-load
tests/kind/run-job.sh affogato/acceptance:dev '@retention'      # 7.1 green
tests/kind/run-job.sh affogato/acceptance:dev '@idempotency'    # 5.1 green; 5.2a/5.2b still pending
```

---

## ACC2. Hot path and finalization

**Status:** planned · **Depends on:** P3, P4, SW4, API2, H2 (Stream Worker + Serving API)
· **Scenarios:** 1.1, 1.3, 1.4, 2.1–2.6, 5.2a

**Scope**
- Steps for Feature 1 (except `@load`), Feature 2, and the Stream Worker half of 5.2.
- This is the first end-to-end playable video: report it with the playlist and a segment
  fetched through the API.

**Verify**
```sh
make -C tests kind-load
tests/kind/run-job.sh affogato/acceptance:dev '@hot-path && ~@load'   # 1.1, 1.3, 1.4 green
tests/kind/run-job.sh affogato/acceptance:dev '@finalization'         # 2.1-2.6 green
tests/kind/run-job.sh affogato/acceptance:dev '@idempotency'          # 5.1 + 5.2a green, 5.2b pending
```

---

## ACC3. Cold path, failures and search

**Status:** planned · **Depends on:** AI3, AI4, API3, H2 (all services)
· **Scenarios:** 3.1, 3.2, 4.1, 4.2, 4.3, 5.2b, 6.1–6.5

**Scope**
- Steps for Feature 3 (except `@load`), Feature 4, the AI half of 5.2, Feature 6.
- Feature 4 includes the long scenarios (AI crash loop ≈ 6 min, retry to DLQ ≈ 2 min,
  redrive ≈ 4 min); the whole non-`@load` run is expected to take ≈ 35 min.

**Verify**
```sh
make -C tests acceptance            # all 23 non-@load scenarios green
```

---

## ACC4. Load scenarios and strict CI

**Status:** planned · **Depends on:** ACC3, SW5, AI4 · **Scenarios:** 1.2, 3.3

**Scope**
- Steps for the two `@load` scenarios; results recorded as indicative (B4 CI 9).
- `.github/workflows/acceptance.yml`: build images, kind, preload, `helm install`, Job with
  `~@load` and `GODOG_STRICT=true`, collect pod logs. `@load` runs nightly or on demand.

**Verify**
```sh
make -C tests load
GODOG_STRICT=true make -C tests acceptance      # strict: any pending step fails the run
gh run list --workflow acceptance.yml
```

**Report includes:** the latency numbers against the targets (500 ms p99 delivery, 1.5 s p95
segment ready, 5 s p95 cold path) and the CI run link.

---

## Scenario status

| # | Scenario | Milestone | Status |
|---|---|---|---|
| 1.1 | Valid video becomes HLS segments and plays as VOD | ACC2 | pending |
| 1.2 | @load hot-path latency | ACC4 | pending |
| 1.3 | Out-of-order chunks | ACC2 | pending |
| 1.4 | Corrupted chunk dead-lettered | ACC2 | pending |
| 2.1 | Playlist before finalization | ACC2 | pending |
| 2.2 | VideoIngestCompleted finalizes | ACC2 | pending |
| 2.3 | Completion before last chunk | ACC2 | pending |
| 2.4 | Idle-timeout finalization | ACC2 | pending |
| 2.5 | No playable chunk | ACC2 | pending |
| 2.6 | Late chunk | ACC2 | pending |
| 3.1 | Speech chunk indexed | ACC3 | pending |
| 3.2 | No speech / no audio | ACC3 | pending |
| 3.3 | @load cold-path latency | ACC4 | pending |
| 4.1 | AI failure retried then DLQ | ACC3 | pending |
| 4.2 | Crash loop broken | ACC3 | pending |
| 4.3 | Redrive | ACC3 | pending |
| 5.1 | Duplicate publish deduped | ACC1 | pending |
| 5.2a | Redelivery, Stream Worker | ACC2 | pending |
| 5.2b | Redelivery, AI Worker | ACC3 | pending |
| 6.1 | Merged ranges | ACC3 | pending |
| 6.2 | Processing videos shown | ACC3 | pending |
| 6.3 | Thumbnail redirect | ACC3 | pending |
| 6.4 | Thumbnail placeholder | ACC3 | pending |
| 6.5 | Legacy path | ACC3 | pending |
| 7.1 | Retention | ACC1 | pending |

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
