#!/usr/bin/env bash
# Forwards the cluster services the host-side runner needs, matching the defaults in
# tests/acceptance/internal/config. Ctrl-C stops all forwards.
# TODO(chart): service names/ports are placeholders until deploy/helm/affogato exists.
set -euo pipefail
ns=${NAMESPACE:-affogato}
kubectl=${KUBECTL:-kubectl}

pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT INT TERM
fwd() { "$kubectl" -n "$ns" port-forward --address 127.0.0.1 "svc/$1" "$2" >/dev/null & pids+=($!); }

fwd affogato-nats        4222:4222
fwd affogato-minio       9000:9000
fwd affogato-qdrant      6333:6333
fwd affogato-serving-api 8080:8080

echo "forwarding NATS :4222, MinIO :9000, Qdrant :6333, API :8080 from namespace $ns (Ctrl-C to stop)"
echo "export AF_MINIO_ACCESS_KEY / AF_MINIO_SECRET_KEY from the MinIO secret before running the tests"
wait
