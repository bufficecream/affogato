#!/usr/bin/env bash
# Verifies every generated media fixture with ffprobe/ffmpeg: containers, codecs, profile,
# resolution, frame/sample rate, durations, keyframe placement, chunk contiguity, audio level,
# the failure modes of the broken chunks, and SHA256SUMS. Prints one line per check and exits
# 1 if any check fails. make_fixtures.sh runs it at the end; run it on its own with:
#
#   tests/fixtures/verify_fixtures.sh [DIR]          (default: tests/fixtures/media)
#   make -C tests fixtures-verify
#
# Env: FFMPEG / FFPROBE as in make_fixtures.sh. With no host ffmpeg the script re-runs itself
# in the pinned ffmpeg container (tests/ffmpeg-docker.sh); FIXTURES_DOCKER=1 forces that.
#      FIXTURES_STRICT_SPEECH=1 fails if the speech set is missing (default: reported, skipped).
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
dir=${1:-"$here/media"}
[ -d "$dir" ] || { echo "error: $dir does not exist; run make_fixtures.sh first" >&2; exit 1; }
dir=$(cd "$dir" && pwd)

if [ -z "${AF_IN_FFMPEG_CONTAINER:-}" ] && [ -z "${FFMPEG:-}" ] &&
   { [ "${FIXTURES_DOCKER:-0}" = 1 ] || ! command -v ffmpeg >/dev/null 2>&1 || ! command -v ffprobe >/dev/null 2>&1; }; then
  command -v docker >/dev/null 2>&1 || { echo "error: neither ffmpeg nor docker found" >&2; exit 1; }
  FFMPEG_DOCKER_MOUNTS=$dir exec "$here/../ffmpeg-docker.sh" bash "$here/verify_fixtures.sh" "$dir"
fi
FFMPEG=${FFMPEG:-ffmpeg}
case "$FFMPEG" in *ffmpeg-docker.sh) FFPROBE=${FFPROBE:-${FFMPEG%ffmpeg-docker.sh}ffprobe-docker.sh} ;; esac
FFPROBE=${FFPROBE:-ffprobe}

cd "$dir"
fails=0 checks=0
pass() { checks=$((checks + 1)); printf 'PASS  %-34s %s\n' "$1" "$2"; }
fail() { checks=$((checks + 1)); fails=$((fails + 1)); printf 'FAIL  %-34s %s\n' "$1" "$2"; }
expect() { # name what actual expected
  if [ "$3" = "$4" ]; then pass "$1" "$2=$3"; else fail "$1" "$2: got '$3', want '$4'"; fi
}
between() { awk -v v="$1" -v lo="$2" -v hi="$3" 'BEGIN { exit !(v != "" && v != "N/A" && v + 0 >= lo && v + 0 <= hi) }'; }
expect_range() { # name what actual lo hi
  if between "$3" "$4" "$5"; then pass "$1" "$2=$3 in [$4,$5]"; else fail "$1" "$2=$3 not in [$4,$5]"; fi
}

# ffprobe one value list (one line per stream/packet), "N/A" kept.
# Probe helpers never fail (pipefail + set -e would abort the run on a broken fixture);
# a failed probe yields empty output and the calling check reports FAIL.
pv() { local f=$1; shift; { "$FFPROBE" -v error "$@" -of default=nw=1:nk=1 "$f" 2>/dev/null | sed '/^$/d'; } || true; }
# MPEG-TS output lists every stream twice (program + stream sections): dedupe by index.
uniq_streams() { "$FFPROBE" -v error "${@:2}" -show_entries stream=index,codec_name -of csv=p=0 "$1" 2>/dev/null |
  sed '/^$/d' | LC_ALL=C sort -t, -k1,1n -u | cut -d, -f2 || true; }
streams() { uniq_streams "$1" | paste -sd' ' -; }
sv() { pv "$1" -select_streams "$2" -show_entries "stream=$3" | head -1 || true; }
fmt() { pv "$1" -show_entries "format=$2"; }
vflags() { pv "$1" -select_streams v:0 -show_entries packet=flags; }
# Key-frame pts_time list of the first video stream, space separated.
keytimes() { "$FFPROBE" -v error -select_streams v:0 -show_entries packet=pts_time,flags -of csv=p=0 "$1" |
  awk -F, '$2 ~ /^K/ { printf "%s%.3f", (n++ ? " " : ""), $1 }' || true; }
# max_volume of the first audio stream in dB ("-inf" for pure zeros).
maxvol() { "$FFMPEG" -nostdin -hide_banner -i "$1" -map 0:a:0 -af volumedetect -f null - 2>&1 |
  awk '/max_volume:/ { print $(NF-1) }' || true; }
probe_fails() { ! "$FFPROBE" -v error -show_streams "$1" >/dev/null 2>&1; }

check_av() { # file w h duration tolerance container
  local f=$1 w=$2 h=$3 d=$4 tol=$5 c=$6
  [ -f "$f" ] || { fail "$f" "missing"; return; }
  expect "$f" container "$(fmt "$f" format_name)" "$c"
  expect "$f" streams "$(streams "$f")" "h264 aac"
  expect "$f" video.profile "$(sv "$f" v:0 profile)" "High"
  expect "$f" video.size "$(sv "$f" v:0 width)x$(sv "$f" v:0 height)" "${w}x${h}"
  expect "$f" video.pix_fmt "$(sv "$f" v:0 pix_fmt)" "yuv420p"
  expect "$f" video.r_frame_rate "$(sv "$f" v:0 r_frame_rate)" "30/1"
  expect "$f" audio.sample_rate "$(sv "$f" a:0 sample_rate)" "48000"
  expect "$f" audio.channels "$(sv "$f" a:0 channels)" "2"
  expect_range "$f" duration "$(fmt "$f" duration)" "$(awk -v d="$d" -v t="$tol" 'BEGIN{print d-t}')" "$(awk -v d="$d" -v t="$tol" 'BEGIN{print d+t}')"
}
check_gop() { # file: key frames exactly every 2.000 s from 0
  local f=$1 d=$2 want="" t
  for ((t = 0; t < d; t += 2)); do want="$want${want:+ }$(printf '%.3f' "$t")"; done
  expect "$f" keyframes_at "$(keytimes "$f")" "$want"
}
check_tone() { # audible 1 kHz tone, not silence
  local v; v=$(maxvol "$1")
  if between "$v" -40 0; then pass "$1" "audio.max_volume=${v} dB (tone)"; else fail "$1" "audio.max_volume=${v} dB, want a tone (> -40 dB)"; fi
}

echo "== verifying fixtures in $dir ($("$FFPROBE" -version 2>/dev/null | head -1 | cut -d' ' -f1-3))"

# Full-length samples (Feature 1 valid video; fast scenarios).
check_av sample_1080p.mp4 1920 1080 10 0.1 "mov,mp4,m4a,3gp,3g2,mj2"
check_gop sample_1080p.mp4 10
check_tone sample_1080p.mp4
check_av sample_short.mp4 640 360 6 0.1 "mov,mp4,m4a,3gp,3g2,mj2"
check_gop sample_short.mp4 6
check_av sample_12s.mp4 640 360 12 0.1 "mov,mp4,m4a,3gp,3g2,mj2"
check_gop sample_12s.mp4 12
check_tone sample_12s.mp4
# moov before mdat (faststart): playable while streaming, as the Producer probes it.
first_atom=$(dd if=sample_1080p.mp4 bs=1 skip=4 count=4 2>/dev/null; dd if=sample_1080p.mp4 bs=1 skip=$((0x$(od -An -tx1 -N4 sample_1080p.mp4 | tr -d ' \n') + 4)) count=4 2>/dev/null)
expect sample_1080p.mp4 "atoms[0..1]" "$first_atom" "ftypmoov"

# Producer-style chunks of sample_12s.mp4: 6 x ~2 s, each one GOP starting on a key frame,
# contiguous timestamps (reset_timestamps 0), stream copy (packet count preserved).
set -- chunks/six/*.ts
expect chunks/six count "$#" 6
prev="" total_pkts=0
for c in "$@"; do
  expect "$c" container "$(fmt "$c" format_name)" mpegts
  expect "$c" streams "$(streams "$c")" "h264 aac"
  first=$(vflags "$c" | head -1)
  case "$first" in K*) pass "$c" "first video packet is a key frame ($first)" ;; *) fail "$c" "first video packet not a key frame ($first)" ;; esac
  expect "$c" keyframe_count "$(vflags "$c" | grep -c '^K' || true)" 1
  n=$(vflags "$c" | wc -l | tr -d ' ')
  expect "$c" video_packets "$n" 60
  total_pkts=$((total_pkts + n))
  expect_range "$c" duration "$(fmt "$c" duration)" 1.95 2.10
  st=$(fmt "$c" start_time)
  if [ -n "$prev" ]; then
    expect_range "$c" start_time_step "$(awk -v a="$st" -v b="$prev" 'BEGIN{printf "%.3f", a-b}')" 1.95 2.05
  fi
  prev=$st
done
expect chunks/six total_video_packets "$total_pkts" "$(vflags sample_12s.mp4 | wc -l | tr -d ' ')"
expect chunks/six/000000.ts "video.size" "$(sv chunks/six/000000.ts v:0 width)x$(sv chunks/six/000000.ts v:0 height)" 640x360

# Broken chunks (Feature 1 corrupted chunk, Feature 2 no playable chunk, edge cases).
if probe_fails chunk_corrupted.ts; then pass chunk_corrupted.ts "ffprobe fails (auto-detect)"; else fail chunk_corrupted.ts "ffprobe accepted it"; fi
if ! "$FFPROBE" -v error -f mpegts -show_streams chunk_corrupted.ts >/dev/null 2>&1; then pass chunk_corrupted.ts "ffprobe -f mpegts fails"; else fail chunk_corrupted.ts "ffprobe -f mpegts accepted it"; fi
expect chunk_corrupted.ts size "$(wc -c < chunk_corrupted.ts | tr -d ' ')" 48000
if probe_fails chunk_empty.ts; then pass chunk_empty.ts "ffprobe fails"; else fail chunk_empty.ts "ffprobe accepted it"; fi
expect chunk_empty.ts size "$(wc -c < chunk_empty.ts | tr -d ' ')" 0
expect chunk_truncated.ts size "$(wc -c < chunk_truncated.ts | tr -d ' ')" 2000
# Documented behaviour, not a wish: ffprobe ACCEPTS the truncated chunk; only a remux fails.
# A worker that validates with ffprobe alone would pass it on (see docs/test-plan.md).
if probe_fails chunk_truncated.ts; then fail chunk_truncated.ts "ffprobe now fails (behaviour changed: update docs)"; else pass chunk_truncated.ts "ffprobe accepts it (known: probe-only validation is not enough)"; fi
if "$FFMPEG" -nostdin -v error -i chunk_truncated.ts -map 0 -c copy -f mpegts -y /dev/null >/dev/null 2>&1; then
  fail chunk_truncated.ts "stream-copy remux succeeded"; else pass chunk_truncated.ts "stream-copy remux fails"; fi

# Cold-path outcome chunks (Feature 3).
f=chunk_silent_audio.ts
expect $f container "$(fmt $f format_name)" mpegts
expect $f streams "$(streams $f)" "h264 aac"
expect_range $f duration "$(fmt $f duration)" 1.95 2.10
expect_range $f audio.duration "$(sv $f a:0 duration)" 1.8 2.1
v=$(maxvol $f)
if [ "$v" = "-inf" ] || between "$v" -200 -90; then pass $f "audio.max_volume=${v} dB (digital silence)"; else fail $f "audio.max_volume=${v} dB, want <= -90"; fi
case "$(vflags $f | head -1)" in K*) pass $f "starts on a key frame" ;; *) fail $f "does not start on a key frame" ;; esac
f=chunk_no_audio.ts
expect $f container "$(fmt $f format_name)" mpegts
expect $f streams "$(streams $f)" "h264"
expect $f audio_streams "$(uniq_streams $f -select_streams a | wc -l | tr -d ' ')" 0
expect_range $f duration "$(fmt $f duration)" 1.95 2.10
case "$(vflags $f | head -1)" in K*) pass $f "starts on a key frame" ;; *) fail $f "does not start on a key frame" ;; esac

# Producer codec gate inputs (A12 / G9).
f=sample_mpeg4_aac.mp4
expect $f streams "$(streams $f)" "mpeg4 aac"
vc=$(sv $f v:0 codec_name)
if [ "$vc" != h264 ]; then pass $f "video codec is not H.264 ($vc, $(sv $f v:0 profile))"; else fail $f "video codec is H.264"; fi
expect_range $f duration "$(fmt $f duration)" 3.9 4.1
f=sample_h264_noaudio.mp4
expect $f streams "$(streams $f)" "h264"
expect $f audio_streams "$(uniq_streams $f -select_streams a | wc -l | tr -d ' ')" 0
expect_range $f duration "$(fmt $f duration)" 3.9 4.1

# Optional speech set (Feature 3 / 6): at least 8 x 2 s chunks of audible speech.
if [ -f speech/speech_data_pipelines_en.mp4 ]; then
  f=speech/speech_data_pipelines_en.mp4
  expect $f streams "$(streams $f)" "h264 aac"
  expect_range $f duration "$(fmt $f duration)" 16 3600
  set -- speech/chunks/*.ts
  if [ "$#" -ge 8 ]; then pass speech/chunks "count=$# (>= 8)"; else fail speech/chunks "count=$#, want >= 8 (Feature 6 merges chunks 0..7)"; fi
  i=0
  for c in "$@"; do
    case "$(vflags "$c" | head -1)" in K*) ;; *) fail "$c" "does not start on a key frame" ;; esac
    if [ $i -lt 8 ]; then
      v=$(maxvol "$c")
      if between "$v" -40 0; then pass "$c" "audible (max_volume=${v} dB)"; else fail "$c" "too quiet for STT (max_volume=${v} dB)"; fi
    fi
    i=$((i + 1))
  done
  cmp -s chunk_speech_en.ts speech/chunks/000000.ts && pass chunk_speech_en.ts "== speech/chunks/000000.ts" || fail chunk_speech_en.ts "differs from speech/chunks/000000.ts"
  echo "NOTE  speech content (English, 'data pipeline' + 'architecture' in every chunk) is not checked here; verify with faster-whisper small"
elif [ "${FIXTURES_STRICT_SPEECH:-0}" = 1 ]; then
  fail speech "speech set missing (FIXTURES_STRICT_SPEECH=1)"
else
  echo "SKIP  speech set                          no recording provided (Feature 3/6 speech scenarios need it)"
fi

# Checksums cover exactly the files present.
if command -v sha256sum >/dev/null 2>&1; then sum=(sha256sum); else sum=(shasum -a 256); fi
if [ -f SHA256SUMS ] && "${sum[@]}" -c --quiet SHA256SUMS >/dev/null 2>&1; then
  listed=$(awk '{print $2}' SHA256SUMS | LC_ALL=C sort | paste -sd' ' -)
  present=$(find . -type f ! -name SHA256SUMS | LC_ALL=C sort | paste -sd' ' -)
  expect SHA256SUMS "covers all files" "$([ "$listed" = "$present" ] && echo yes || echo no)" yes
else
  fail SHA256SUMS "missing or does not verify"
fi

echo "== $((checks - fails))/$checks checks passed"
[ "$fails" -eq 0 ]
