#!/usr/bin/env bash
# Runs `go` in the official golang container for machines without a local toolchain.
# Mounts the repository and keeps module/build caches in named volumes.
#   make -C tests unit GO="$PWD/tests/go-docker.sh"
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
rel=${PWD#"$root"}
exec docker run --rm -i \
  -v "$root:/repo" -v af-gomod:/go/pkg/mod -v af-gocache:/root/.cache/go-build \
  -e NO_COLOR -e GODOG_TAGS -e GODOG_STRICT -e GODOG_FORMAT \
  -w "/repo$rel" "golang:${GO_VERSION:-1.26}-bookworm" go "$@"
