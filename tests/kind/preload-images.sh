#!/usr/bin/env bash
# Pulls the images in images.txt once (skipped when already present locally) and loads them
# into the kind cluster. Usage: preload-images.sh [CLUSTER]   Env: KIND (kind), DOCKER (docker)
set -euo pipefail

cluster=${1:-affogato}
kind=${KIND:-kind}
docker=${DOCKER:-docker}
here=$(cd "$(dirname "$0")" && pwd)
# kind's node platform. `kind load docker-image` imports --all-platforms and fails on
# multi-arch images pulled for one platform only, so save just this one and load the archive.
platform=${PLATFORM:-linux/$("$docker" version -f '{{.Server.Arch}}')}
tmp=$(mktemp -d) && trap 'rm -rf "$tmp"' EXIT

grep -v -E '^\s*(#|$)' "$here/images.txt" | while read -r ref; do
  if ! "$docker" image inspect "$ref" >/dev/null 2>&1; then
    echo "-> pull $ref"
    "$docker" pull -q --platform "$platform" "$ref" >/dev/null
  fi
  # Save by tag: an archive saved by digest reference is imported without a name.
  tag=${ref%@*} want=${ref#*@}
  got=$("$docker" image inspect -f '{{range .RepoDigests}}{{println .}}{{end}}' "$tag" | sed -n 's/.*@//p')
  grep -qx "$want" <<<"$got" || { echo "digest mismatch for $tag: want $want, have: $got" >&2; exit 1; }
  echo "-> kind load $tag ($platform)"
  "$docker" save --platform "$platform" -o "$tmp/image.tar" "$tag"
  "$kind" load image-archive "$tmp/image.tar" --name "$cluster"
done
