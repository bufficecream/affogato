#!/usr/bin/env bash
# Forwards the cluster services the host-side runner needs, matching the defaults in
# tests/acceptance/internal/config. Ctrl-C stops all forwards.
# TODO(chart): service names/ports are placeholders until deploy/helm/framepulse exists.
set -euo pipefail
ns=${NAMESPACE:-framepulse}
kubectl=${KUBECTL:-kubectl}

pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true' EXIT INT TERM
fwd() { "$kubectl" -n "$ns" port-forward --address 127.0.0.1 "svc/$1" "$2" >/dev/null & pids+=($!); }

fwd framepulse-nats        4222:4222
fwd framepulse-minio       9000:9000
fwd framepulse-qdrant      6333:6333
fwd framepulse-serving-api 8080:8080

echo "forwarding NATS :4222, MinIO :9000, Qdrant :6333, API :8080 from namespace $ns (Ctrl-C to stop)"
echo "export FP_MINIO_ACCESS_KEY / FP_MINIO_SECRET_KEY from the MinIO secret before running the tests"
wait
