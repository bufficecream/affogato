#!/usr/bin/env bash
# Runs ffmpeg / ffprobe (or a script that needs them) in a pinned container, for machines
# without a local ffmpeg. Mirrors go-docker.sh.
#
#   tests/ffmpeg-docker.sh -version                       # ffmpeg (default command)
#   tests/ffmpeg-docker.sh ffprobe -v error -show_streams tests/fixtures/media/x.ts
#   tests/ffprobe-docker.sh -v error -show_streams ...     # symlink: always ffprobe
#   tests/ffmpeg-docker.sh bash tests/fixtures/make_fixtures.sh
#   make -C tests fixtures FFMPEG="$PWD/tests/ffmpeg-docker.sh"   # one container per call
#
# The repository and the current directory are mounted at their host paths, so absolute and
# relative paths work unchanged. Absolute-path arguments outside them get their parent
# directory mounted too; FFMPEG_DOCKER_MOUNTS adds more (space-separated directories).
# Files are written as the calling user. Override the image with FFMPEG_IMAGE.
set -euo pipefail

# linuxserver/ffmpeg 7.1.1 (Ubuntu based, amd64+arm64, libx264, bash). Tag + index digest.
FFMPEG_IMAGE=${FFMPEG_IMAGE:-linuxserver/ffmpeg:version-7.1.1-cli@sha256:aea59a11c54291ac456bb2d67000445e5a8994f70bc3d96cdc29f022fbbf89fb}

root=$(cd "$(dirname "$0")/.." && pwd)
here=$(pwd)

case "$(basename "$0")" in
  ffprobe-docker.sh) cmd=ffprobe ;;
  *)
    case "${1:-}" in
      ffmpeg|ffprobe|bash|sh) cmd=$1; shift ;;
      *) cmd=ffmpeg ;;
    esac ;;
esac

mounts=("$root")
case "$here/" in "$root"/*) ;; *) mounts+=("$here") ;; esac
add_mount() { # $1 directory (absolute); skipped if already covered by a mount
  local d=$1 m
  [ -d "$d" ] || return 0
  d=$(cd "$d" && pwd)
  for m in "${mounts[@]}"; do case "$d/" in "$m"/*) return 0 ;; esac; done
  mounts+=("$d")
}
for a in "$@"; do
  case "$a" in /*) if [ -d "$a" ]; then add_mount "$a"; else add_mount "$(dirname "$a")"; fi ;; esac
done
for d in ${FFMPEG_DOCKER_MOUNTS:-}; do add_mount "$d"; done

args=(--rm -i --user "$(id -u):$(id -g)" -e HOME=/tmp --entrypoint "$cmd" -w "$here")
for m in "${mounts[@]}"; do args+=(-v "$m:$m"); done
# Pass through the knobs make_fixtures.sh / verify_fixtures.sh read.
for v in SPEECH_SRC FIXTURES_STRICT_SPEECH; do [ -n "${!v:-}" ] && args+=(-e "$v"); done
args+=(-e FP_IN_FFMPEG_CONTAINER=1)

exec docker run "${args[@]}" "$FFMPEG_IMAGE" "$@"
