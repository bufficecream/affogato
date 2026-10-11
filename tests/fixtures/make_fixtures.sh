#!/usr/bin/env bash
# Generates the acceptance-test media fixtures with ffmpeg. Outputs are NOT committed
# (tests/fixtures/media/ is git-ignored); run this locally, in CI before building the
# runner image, or let the runner image Dockerfile run it in its build stage.
#
# Usage: tests/fixtures/make_fixtures.sh [OUT_DIR]      (default: tests/fixtures/media)
# Env:   FFMPEG / FFPROBE override binaries (e.g. FFMPEG=tests/ffmpeg-docker.sh, one container
#        per call); unset and no host ffmpeg -> the whole script re-runs in the pinned ffmpeg
#        container (tests/ffmpeg-docker.sh); FIXTURES_DOCKER=1 forces that.
#        SPEECH_SRC overrides the speech input file. Verification: verify_fixtures.sh.
#
# Fixtures (all speech-free except the optional speech set):
#   sample_1080p.mp4            1920x1080 30 fps H.264 High + AAC 48 kHz, 1 kHz tone, 10 s,
#                               fixed 2 s GOP -> 5 chunks.  Feature 1 "valid video".
#   sample_short.mp4            640x360 H.264 + AAC tone, 6 s -> 3 chunks (fast scenarios).
#   sample_12s.mp4              640x360 H.264 + AAC tone, 12 s -> 6 chunks.
#   chunks/six/000000..000005.ts  sample_12s.mp4 cut exactly like the Producer
#                               (ffmpeg -c copy -f segment -segment_format mpegts -segment_time 2
#                               -segment_time_delta 0.1);
#                               scenarios that publish events directly upload these.
#   chunk_corrupted.ts          bytes that are not a media container (ffprobe must fail).
#   chunk_truncated.ts          first 2000 bytes of a valid chunk (edge case; not 188-aligned).
#   chunk_empty.ts              0 bytes (edge case).
#   chunk_silent_audio.ts       2 s H.264 + AAC digital silence  -> NO_SPEECH.
#   chunk_no_audio.ts           2 s H.264, no audio stream       -> NO_AUDIO.
#   sample_mpeg4_aac.mp4        MPEG-4 Part 2 video + AAC        -> Producer must reject (A12).
#   sample_h264_noaudio.mp4     H.264 without audio              -> Producer behaviour undefined (spec gap).
#   speech/...                  only if a speech recording is provided, see README.md.
#   SHA256SUMS                  checksums of everything above.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
out=${1:-"$here/media"}
speech_src=${SPEECH_SRC:-}

# No ffmpeg on the host (and no FFMPEG override)? Re-run this whole script once inside the
# pinned ffmpeg container (tests/ffmpeg-docker.sh): one container instead of one per call.
# FIXTURES_DOCKER=1 forces the container even when a host ffmpeg exists.
if [ -z "${AF_IN_FFMPEG_CONTAINER:-}" ] && [ -z "${FFMPEG:-}" ] &&
   { [ "${FIXTURES_DOCKER:-0}" = 1 ] || ! command -v ffmpeg >/dev/null 2>&1 || ! command -v ffprobe >/dev/null 2>&1; }; then
  wrapper="$here/../ffmpeg-docker.sh"
  command -v docker >/dev/null 2>&1 || { echo "error: neither ffmpeg nor docker found" >&2; exit 1; }
  echo "-> no host ffmpeg (or FIXTURES_DOCKER=1): running in container via tests/ffmpeg-docker.sh"
  mkdir -p "$out"; out=$(cd "$out" && pwd)
  mounts="$out"
  if [ -n "$speech_src" ]; then speech_src=$(cd "$(dirname "$speech_src")" && pwd)/$(basename "$speech_src"); mounts="$mounts $(dirname "$speech_src")"; fi
  SPEECH_SRC=$speech_src FFMPEG_DOCKER_MOUNTS=$mounts exec "$wrapper" bash "$here/make_fixtures.sh" "$out"
fi

FFMPEG=${FFMPEG:-ffmpeg}
# A wrapper given as FFMPEG implies its ffprobe sibling (ffmpeg-docker.sh -> ffprobe-docker.sh).
case "$FFMPEG" in *ffmpeg-docker.sh) FFPROBE=${FFPROBE:-${FFMPEG%ffmpeg-docker.sh}ffprobe-docker.sh} ;; esac
FFPROBE=${FFPROBE:-ffprobe}
export FFMPEG FFPROBE

for bin in "$FFMPEG" "$FFPROBE"; do
  command -v "$bin" >/dev/null 2>&1 || { echo "error: $bin not found (install ffmpeg or use tests/ffmpeg-docker.sh)" >&2; exit 1; }
done
# Capture first: `ffmpeg | grep -q` dies with SIGPIPE (141) under pipefail when grep exits early.
encoders=$("$FFMPEG" -hide_banner -encoders 2>/dev/null)
case "$encoders" in *libx264*) ;; *) echo "error: ffmpeg lacks libx264" >&2; exit 1 ;; esac

mkdir -p "$out/chunks/six"
cd "$out"
# Drop outputs of earlier runs that this run might not overwrite (chunk counts, speech set).
rm -rf chunks/six/*.ts speech chunk_speech_en.ts SHA256SUMS

# Common flags: reproducible output, no source metadata, quiet.
ff() { "$FFMPEG" -hide_banner -loglevel error -nostdin -y "$@"; }
BITEXACT=(-fflags +bitexact -flags:v +bitexact -flags:a +bitexact -map_metadata -1)
# Fixed 2 s GOP at 30 fps so every cut lands on an IDR (spec: chunks start on a keyframe).
H264=(-c:v libx264 -preset veryfast -profile:v high -pix_fmt yuv420p -r 30 -g 60 -keyint_min 60 -sc_threshold 0 -x264-params threads=1)
AAC=(-c:a aac -b:a 128k -ar 48000 -ac 2)

tone() { echo "sine=frequency=1000:sample_rate=48000:duration=$1"; }
video() { echo "testsrc2=size=$1:rate=30:duration=$2"; }

echo "-> sample_1080p.mp4"
ff -f lavfi -i "$(video 1920x1080 10)" -f lavfi -i "$(tone 10)" "${H264[@]}" "${AAC[@]}" \
   "${BITEXACT[@]}" -movflags +faststart -shortest sample_1080p.mp4

echo "-> sample_short.mp4 / sample_12s.mp4"
ff -f lavfi -i "$(video 640x360 6)" -f lavfi -i "$(tone 6)" "${H264[@]}" "${AAC[@]}" \
   "${BITEXACT[@]}" -movflags +faststart -shortest sample_short.mp4
ff -f lavfi -i "$(video 640x360 12)" -f lavfi -i "$(tone 12)" "${H264[@]}" "${AAC[@]}" \
   "${BITEXACT[@]}" -movflags +faststart -shortest sample_12s.mp4

# -segment_time_delta: without it the segment muxer misses the cut at 2.000 s when the input
# has B-frames (dts starts below 0), so chunk 0 becomes ~4 s and a 12 s input yields 5 chunks
# (seen with ffmpeg 7.1.1). The Producer needs the same option (reported to the developer).
cut_chunks() { # $1 input, $2 output dir
  ff -i "$1" -map 0 -c copy -f segment -segment_format mpegts -segment_time 2 \
     -segment_time_delta 0.1 -reset_timestamps 0 "${BITEXACT[@]}" "$2/%06d.ts"
}
echo "-> chunks/six/*.ts"
cut_chunks sample_12s.mp4 chunks/six
n=$(ls chunks/six/*.ts | wc -l | tr -d ' ')
[ "$n" -eq 6 ] || { echo "error: expected 6 chunks from sample_12s.mp4, got $n" >&2; exit 1; }

echo "-> chunk_corrupted.ts / chunk_truncated.ts / chunk_empty.ts"
# Deterministic noise as raw PCM: no demuxer recognises it, so ffprobe fails (INVALID_MEDIA).
ff -f lavfi -i "anoisesrc=duration=0.5:sample_rate=48000:seed=42:color=white" -f s16le -c:a pcm_s16le chunk_corrupted.ts
head -c 2000 chunks/six/000000.ts > chunk_truncated.ts
: > chunk_empty.ts

echo "-> chunk_silent_audio.ts / chunk_no_audio.ts"
ff -f lavfi -i "$(video 640x360 2)" -f lavfi -i "anullsrc=channel_layout=stereo:sample_rate=48000" \
   -t 2 "${H264[@]}" "${AAC[@]}" "${BITEXACT[@]}" -f mpegts chunk_silent_audio.ts
ff -f lavfi -i "$(video 640x360 2)" -an "${H264[@]}" "${BITEXACT[@]}" -f mpegts chunk_no_audio.ts

echo "-> sample_mpeg4_aac.mp4 / sample_h264_noaudio.mp4"
ff -f lavfi -i "$(video 640x360 4)" -f lavfi -i "$(tone 4)" -c:v mpeg4 -q:v 5 -g 60 "${AAC[@]}" \
   "${BITEXACT[@]}" -shortest sample_mpeg4_aac.mp4
ff -f lavfi -i "$(video 640x360 4)" -an "${H264[@]}" "${BITEXACT[@]}" sample_h264_noaudio.mp4

# Optional speech set (Feature 3 / Feature 6). The recording itself must be provided.
if [ -z "$speech_src" ]; then
  for c in "$here"/speech/data_pipelines_en.wav "$here"/speech/data_pipelines_en.m4a "$here"/speech/data_pipelines_en.aiff "$here"/speech/data_pipelines_en.mp3; do
    [ -f "$c" ] && { speech_src=$c; break; }
  done
fi
if [ -n "$speech_src" ]; then
  echo "-> speech set from $speech_src"
  mkdir -p speech/chunks
  dur=$("$FFPROBE" -v error -show_entries format=duration -of csv=p=0 "$speech_src")
  ff -f lavfi -i "testsrc2=size=640x360:rate=30" -i "$speech_src" -map 0:v -map 1:a -t "$dur" \
     "${H264[@]}" "${AAC[@]}" "${BITEXACT[@]}" -movflags +faststart speech/speech_data_pipelines_en.mp4
  cut_chunks speech/speech_data_pipelines_en.mp4 speech/chunks
  cp speech/chunks/000000.ts chunk_speech_en.ts
else
  echo "!! no speech recording found: Feature 3/6 speech fixtures skipped (see tests/fixtures/README.md)"
fi

if command -v sha256sum >/dev/null 2>&1; then sum=(sha256sum); else sum=(shasum -a 256); fi
find . -type f ! -name SHA256SUMS | LC_ALL=C sort | xargs "${sum[@]}" > SHA256SUMS
"$here/verify_fixtures.sh" "$out"
echo "fixtures written to $out"
