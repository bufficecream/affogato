# Helm chart milestones (H0–H3)

Chart `deploy/helm/affogato`: the same chart is installed on kind (`values-kind.yaml`) and in
production (`values-prod.yaml`). It owns the infrastructure configuration (streams, consumers,
KV, buckets, collection); services only check that it exists.

Source of truth: [`docs/architecture-review.md`](../../../docs/architecture-review.md)
decisions 13, 15, 16, 17 and B4 CI steps 7–8; [`tests/README.md`](../../../tests/README.md) §6
(what `values-kind.yaml` must set) and §7 (names the runner expects).

Images for kind are preloaded with `make -C tests kind-preload` (`tests/kind/images.txt`) and
must be referenced **by tag only** with `imagePullPolicy: IfNotPresent` in `values-kind.yaml`.

---

## H0. Object store decision

**Status:** planned · **Depends on:** decision D1 (yours) · **Parallel with:** C1–C3, AI0

**Scope**
- Write down the choice (MinIO built from source, a community image, or another S3-compatible
  store) with: image source and licence, arm64 + amd64 availability, bucket lifecycle support
  (`af-raw` 10-day expiry, abort multipart after 1 day), path-style presigned GET, and how
  credentials reach the runner (`AF_MINIO_ACCESS_KEY`/`SECRET_KEY`).
- If the store is not MinIO: decide whether the `AF_MINIO_*` variable names and the
  `affogato-minio` Secret keep their names (cheapest) or are renamed in the runner, the Job and
  the docs.
- Add the chosen image to `tests/kind/images.txt`.

**Verify**
A throwaway container of the chosen image: create both buckets, set the lifecycle rule, PUT
and presigned GET an object, and read the lifecycle rule back. Commands and output go in the
report.

**Report includes:** the decision record (here, under Implementation log) and the smoke-test
output.

---

## H1. Infrastructure chart on kind

**Status:** planned · **Depends on:** H0, C3 · **Unblocks:** ACC1, all service work on kind

**Scope**
- Subcharts, pinned: NATS (JetStream, 1 replica on kind), NACK (applies
  `contracts/nats/**/*.json` as CRDs), the object store, Qdrant. KEDA is added in H2.
- Init Job: buckets + lifecycle (`af-raw` 10 d, `af-media` none), Qdrant collection
  `transcripts_me5s384_v1` + alias `transcripts` from `contracts/qdrant/`.
- The `affogato-minio` Secret (keys `rootUser`, `rootPassword`) the runner Job reads.
- `values-kind.yaml` with stream replicas overridden to 1.

**Verify**
```sh
helm lint deploy/helm/affogato
helm template affogato deploy/helm/affogato -f deploy/helm/affogato/values-kind.yaml | docker run --rm -i ghcr.io/yannh/kubeconform:v0.6.7 -strict -summary -ignore-missing-schemas
make -C tests kind-up kind-preload
helm install affogato deploy/helm/affogato -n affogato -f deploy/helm/affogato/values-kind.yaml --wait
kubectl -n affogato get pods        # all Running/Completed, no image pulls for preloaded images
```
Then ACC1 runs 5.1 and 7.1 against it.

**Report includes:** `helm install` duration; any image that was still pulled from the network.

---

## H2. Service templates and `values-kind.yaml`

**Status:** planned · **Depends on:** H1 and each service's image (SW5, API4, AI4; P4 for the runner)

**Scope**
- Deployments for `affogato-stream-worker`, `affogato-ai-worker`, `affogato-embedder`,
  `affogato-serving-api` (names match `AF_WORKLOAD_*` defaults in `tests/README.md` §7, or the
  defaults are updated), Services, probes, resources.
- `values-kind.yaml`: `FINALIZE_IDLE_TIMEOUT=10s`, `FINALIZE_SWEEP_INTERVAL=2s`,
  `testFaults.enabled: true` (renders `AF_TEST_FAULTS=1` into both workers),
  `MINIO_PUBLIC_ENDPOINT` equal to the Job's `AF_MINIO_PUBLIC_ENDPOINT`.
- KEDA ScaledObjects on consumer `num_pending` for both workers.
- Each service template is added as soon as that service's image exists, so the hot path can
  run on kind before the AI Worker is done.

**Verify**
```sh
helm upgrade --install affogato deploy/helm/affogato -n affogato -f deploy/helm/affogato/values-kind.yaml --wait
kubectl -n affogato get deploy -o wide
make -C tests acceptance            # results tracked in tests/acceptance/MILESTONES.md
```

**Report includes:** which services are deployed, their env as rendered, the acceptance result.

---

## H3. Production values and chart CI

**Status:** planned · **Depends on:** H2

**Scope**
- `values-prod.yaml`: replicas 3 for streams/KV, production timeouts (`5m` / `30s`), resource
  requests, no test faults.
- `.github/workflows/chart.yml` (B4 CI 7): `helm lint`, `helm template | kubeconform` for both
  values files, and a check that `helm template -f values-prod.yaml` does not contain
  `AF_TEST_FAULTS`.

**Verify**
```sh
helm template affogato deploy/helm/affogato -f deploy/helm/affogato/values-prod.yaml | grep -c AF_TEST_FAULTS   # must print 0
gh run list --workflow chart.yml
```

**Report includes:** the CI run link; the prod/kind values diff summary.

---

## Implementation log

_Empty. Each finished milestone adds a report here in the format from
[`docs/implementation-plan.md`](../../../docs/implementation-plan.md#milestone-report-what-you-get-after-each-milestone)._
