# Acceptance test fixtures

Media fixtures are generated, not committed. Run:

```sh
make -C tests fixtures                     # writes tests/fixtures/media/ (git-ignored)
make -C tests fixtures-verify              # re-check existing fixtures only
```

`make_fixtures.sh` uses a host `ffmpeg`/`ffprobe` with `libx264` if there is one;
otherwise it re-runs itself inside the pinned image
`linuxserver/ffmpeg:version-7.1.1-cli@sha256:aea59a11…` through `tests/ffmpeg-docker.sh`
(Docker only). `FIXTURES_DOCKER=1` forces the container; `FFMPEG=$PWD/tests/ffmpeg-docker.sh`
runs one container per ffmpeg call instead. The output is bit-reproducible with the
pinned image (same `SHA256SUMS` on every run and inside the runner image).

After generating, `verify_fixtures.sh` checks every file with ffprobe (103 checks without
the speech set): container, codecs, H.264 profile, size, pixel format, frame/sample rate,
durations, key frames exactly every 2 s, 6 contiguous one-GOP chunks with 60 video packets
each, the tone is audible, the silent chunk is digital silence (≤ -90 dB), the no-audio
files have no audio stream, the MPEG-4 sample is not H.264, ffprobe rejects the corrupted
and empty chunks, and `SHA256SUMS` covers exactly the files present. Any failure fails
`make fixtures` and the runner image build.

Known behaviour the verifier pins down (reported to the developer):
- `chunk_truncated.ts` (first 2000 bytes of a valid chunk) **passes ffprobe** (exit 0,
  h264 + aac); only the stream-copy remux fails. Probe-only validation is not enough.
- Cutting with plain `-segment_time 2` misses the first 2 s boundary on inputs with
  B-frames (chunk 0 ≈ 4 s). The script adds `-segment_time_delta 0.1`; the Producer needs
  the same.

| File | Used by |
|---|---|
| `sample_1080p.mp4` (H.264 High + AAC, 10 s, 1 kHz tone, faststart) | Feature 1 valid video |
| `sample_short.mp4`, `sample_12s.mp4`, `chunks/six/*.ts` | out-of-order, corrupted-chunk, finalization scenarios |
| `chunk_corrupted.ts` | Feature 1 corrupted chunk, Feature 2 "no playable chunk" |
| `chunk_silent_audio.ts`, `chunk_no_audio.ts` | Feature 3 NO_SPEECH / NO_AUDIO |
| `sample_mpeg4_aac.mp4`, `sample_h264_noaudio.mp4` | Producer rejection (A12), proposed scenarios |
| `chunk_truncated.ts`, `chunk_empty.ts` | proposed edge-case scenarios (docs/test-plan.md) |

## Speech fixture (must be provided)

Feature 3 ("English speech about data pipelines") and Feature 6 (transcripts discussing
"data pipeline architecture") need real English speech. It can't be synthesised with
ffmpeg alone, so provide a recording and place it at:

```
tests/fixtures/speech/data_pipelines_en.wav     (or .m4a / .aiff / .mp3)
```

or point `SPEECH_SRC=/path/to/file` at it (paths outside the repository work with the
container too: the script mounts the file's directory). Requirements:

- Clear English speech, mono or stereo, at least 16 s long, so that chunks 0..7 carry speech
  (Feature 6 merges 8 adjacent chunks).
- The words "data pipeline" and "architecture" must be spoken in **every** 2 s window,
  because each chunk is transcribed on its own (Feature 6 expects every chunk 42..49 to
  match). Chunks are cut every 2 s from the start, so the reliable way is one complete
  phrase per 2 s slot: "Data pipeline architecture.", padded with silence to exactly 2.0 s
  and repeated. Continuous prose does not work: a longer sentence spreads the words over
  several chunks and splits words at the boundaries (checked 2026-10-12: 2/8 chunks passed).
- Licence: record it yourself or use TTS output you are allowed to redistribute
  internally. Don't commit it unless the licence allows; keep it under 2 MB.

Ways to produce it:

```sh
# 1. One phrase, about 1.45 s (macOS; on Linux use piper or espeak-ng -w instead of say)
say -v Samantha -r 200 -o tests/data/phrase.aiff "Data pipeline architecture."
# 2. 0.25 s lead-in, pad to exactly 2.0 s, repeat 18 times (36 s, 18 chunks)
tests/ffmpeg-docker.sh -y -i tests/data/phrase.aiff -af "adelay=250:all=1,apad=whole_dur=2" \
  -t 2 -ar 22050 -ac 1 tests/data/phrase_2s.wav
tests/ffmpeg-docker.sh -y -stream_loop 17 -i tests/data/phrase_2s.wav -c:a pcm_s16be \
  tests/fixtures/speech/data_pipelines_en.aiff
```

The phrase must end well before 2.0 s, because chunk lengths vary by about ±0.04 s
(1.96 to 2.01 s). Check the margins with
`tests/ffmpeg-docker.sh -i tests/data/phrase_2s.wav -af silencedetect=n=-40dB:d=0.1 -f null -`.
The clip above has about 0.26 s of silence before the phrase and 0.31 s after it.

The script then builds `speech/speech_data_pipelines_en.mp4`, cuts it into
`speech/chunks/*.ts` like the Producer would, and copies the first chunk to
`chunk_speech_en.ts`. Before relying on a clip, check every chunk with faster-whisper
`small` (`make -C tests tools-image` once, then):

```sh
docker run --rm -v af-whisper-cache:/root/.cache \
  -v "$PWD/tests/fixtures/media/speech/chunks:/chunks:ro" affogato/tools:dev \
  python -I /tools/transcribe_chunks.py /chunks 0 7
```

It fails unless each chunk is detected as `en` and contains both words. The clip above
passed 8/8 ("Data Pipeline Architecture", en 0.98) on 2026-10-12. Robotic espeak voices
sometimes come out with the wrong language on 2 s clips (spec A17).
