# Acceptance test fixtures

Media fixtures are generated, not committed. Run:

```sh
tests/fixtures/make_fixtures.sh            # writes tests/fixtures/media/ (git-ignored)
```

`make_fixtures.sh` needs `ffmpeg` and `ffprobe` with `libx264`. It generates the
speech-free fixtures, checks the properties the scenarios rely on (codecs, keyframe-aligned
chunks, ffprobe rejecting the corrupted chunk) and writes `media/SHA256SUMS`. The runner
image (`tests/acceptance/Dockerfile`) runs the same script in its build stage, so the
in-cluster Job always has the fixtures.

| File | Used by |
|---|---|
| `sample_1080p.mp4` (H.264 High + AAC, 10 s, 1 kHz tone) | Feature 1 valid video |
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

or point `SPEECH_SRC=/path/to/file` at it. Requirements:

- Clear English speech, mono or stereo, at least 16 s long, so that chunks 0..7 carry speech
  (Feature 6 merges 8 adjacent chunks).
- The words "data pipeline" and "architecture" must be spoken in **every** 2 s window,
  because each chunk is transcribed on its own (Feature 6 expects every chunk 42..49 to
  match). Suggested script, read at a normal pace and repeated:
  "Data pipeline architecture. A data pipeline moves events from ingestion to storage.
  This data pipeline architecture uses streams and workers."
- Licence: record it yourself or use TTS output you are allowed to redistribute
  internally. Don't commit it unless the licence allows; keep it under 2 MB.

Ways to produce it:

```sh
# macOS
say -v Samantha -r 170 -o tests/fixtures/speech/data_pipelines_en.aiff \
  "Data pipeline architecture. A data pipeline moves events from ingestion to storage. ..."
# Linux
espeak-ng -v en-us -s 150 -w tests/fixtures/speech/data_pipelines_en.wav "Data pipeline architecture. ..."
# or piper (better quality): echo "..." | piper --model en_US-lessac-medium --output_file .../data_pipelines_en.wav
```

The script then builds `speech/speech_data_pipelines_en.mp4`, cuts it into
`speech/chunks/*.ts` like the Producer would, and copies the first chunk to
`chunk_speech_en.ts`. Before relying on a TTS clip, check that faster-whisper `small`
transcribes it with the expected words and detects `en`. Robotic espeak voices sometimes
come out with the wrong language on 2 s clips (spec A17).
