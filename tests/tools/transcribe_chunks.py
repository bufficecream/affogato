"""Check the speech fixture chunks with faster-whisper (tests/fixtures/README.md).

Usage: transcribe_chunks.py CHUNK_DIR FIRST LAST [MODEL]

Transcribes each chunk {FIRST..LAST:06d}.ts on its own, like the AI Worker does, and
fails unless every chunk is detected as English and contains "data pipeline" and
"architecture" (spec Features 3 and 6).
"""

import subprocess
import sys

import numpy as np
from faster_whisper import WhisperModel

RATE = 16000
WORDS = ("data pipeline", "architecture")


def decode(path):
    # ffmpeg CLI instead of PyAV, see the Dockerfile.
    raw = subprocess.run(
        ["ffmpeg", "-v", "error", "-i", path, "-vn", "-ac", "1", "-ar", str(RATE), "-f", "s16le", "-"],
        capture_output=True,
        check=True,
    ).stdout
    return np.frombuffer(raw, np.int16).astype(np.float32) / 32768


def main(argv):
    if len(argv) not in (4, 5):
        sys.exit(__doc__)
    chunk_dir, first, last = argv[1], int(argv[2]), int(argv[3])
    model = WhisperModel(argv[4] if len(argv) == 5 else "small", device="cpu", compute_type="int8")

    failed = 0
    for i in range(first, last + 1):
        audio = decode(f"{chunk_dir}/{i:06d}.ts")
        segments, info = model.transcribe(audio, beam_size=5)
        text = " ".join(s.text.strip() for s in segments)
        ok = info.language == "en" and all(w in text.lower() for w in WORDS)
        failed += not ok
        print(f"{'PASS' if ok else 'FAIL'}  {i:06d}.ts  {len(audio) / RATE:.2f}s  "
              f"lang={info.language}({info.language_probability:.2f})  {text!r}", flush=True)
    print(f"== {last - first + 1 - failed}/{last - first + 1} chunks passed")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
