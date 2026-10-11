# Local tool image for test-media checks (make -C tests tools-image). Built once, so ffmpeg and
# the Python packages are not downloaded again on every run. Not used by CI or the runner image.
#
#   docker run --rm -v af-whisper-cache:/root/.cache \
#     -v "$PWD/tests/fixtures/media/speech/chunks:/chunks:ro" affogato/tools:dev \
#     python /tools/transcribe_chunks.py /chunks 0 7
#
# The Whisper weights are not baked in: they are downloaded on first use into the
# af-whisper-cache volume and reused from there.
FROM python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1

RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg \
 && rm -rf /var/lib/apt/lists/*

# faster-whisper decodes through PyAV, and current PyAV dropped the `metadata_errors`
# argument it passes. transcribe_chunks.py decodes with the ffmpeg CLI instead.
RUN pip install --no-cache-dir --root-user-action=ignore faster-whisper==1.2.1 requests

COPY transcribe_chunks.py /tools/transcribe_chunks.py
