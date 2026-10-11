#!/usr/bin/env bash
# Runs the acceptance image as an in-cluster Job, streams its logs and exits with the
# Job's result. Usage: run-job.sh IMAGE TAGS [JOB_NAME]
#   IMAGE     e.g. affogato/acceptance:dev (already `kind load`ed)
#   TAGS      godog tag expression, e.g. '~@load' or '@load'
# Env: NAMESPACE (affogato), GODOG_STRICT (false), KUBECTL (kubectl), WAIT_START (300s)
set -euo pipefail

image=${1:?image}
tags=${2:?tags}
job=${3:-af-acceptance}
ns=${NAMESPACE:-affogato}
strict=${GODOG_STRICT:-false}
kubectl=${KUBECTL:-kubectl}
here=$(cd "$(dirname "$0")" && pwd)

case "$job" in *[!a-z0-9-]*|"") echo "invalid job name: $job" >&2; exit 2 ;; esac
case "$strict" in true|false) ;; *) echo "GODOG_STRICT must be true|false" >&2; exit 2 ;; esac
# godog tag syntax: @tag, ~@tag (not), "," (or), "&&" (and). Nothing else is allowed.
case "$tags" in *[!-a-zA-Z0-9@~,\&\ ]*) echo "invalid tag expression: $tags" >&2; exit 2 ;; esac
tags_sed=${tags//&/\\&}   # '&' is special in sed replacements

"$kubectl" apply -f "$here/acceptance-rbac.yaml"
"$kubectl" -n "$ns" delete job "$job" --ignore-not-found --wait=true

sed -e "s|__JOB_NAME__|$job|g" -e "s|__IMAGE__|$image|g" \
    -e "s|__GODOG_TAGS__|$tags_sed|g" -e "s|__GODOG_STRICT__|$strict|g" \
    "$here/acceptance-job.yaml" | "$kubectl" apply -f -

# Stream logs once the pod has started (fails fast on image pull errors via the timeout).
"$kubectl" -n "$ns" wait pod -l "job-name=$job" --for=jsonpath='{.status.phase}'=Running \
  --timeout="${WAIT_START:-300s}" 2>/dev/null || true
"$kubectl" -n "$ns" logs -f "job/$job" --pod-running-timeout=5m || true

for _ in $(seq 1 60); do
  succeeded=$("$kubectl" -n "$ns" get job "$job" -o jsonpath='{.status.succeeded}')
  failed=$("$kubectl" -n "$ns" get job "$job" -o jsonpath='{.status.failed}')
  if [ "${succeeded:-0}" -ge 1 ]; then echo "acceptance job succeeded"; exit 0; fi
  if [ "${failed:-0}" -ge 1 ]; then
    echo "acceptance job FAILED" >&2
    "$kubectl" -n "$ns" describe job "$job" | tail -20 >&2
    exit 1
  fi
  sleep 5   # polling the Job status only; test logic never sleeps
done
echo "acceptance job did not finish after logs ended" >&2
exit 1
