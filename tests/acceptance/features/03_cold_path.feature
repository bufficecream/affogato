# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@cold-path
Feature: Asynchronous transcription and vector indexing
  As a viewer
  I want spoken audio to be transcribed and indexed automatically
  So that I can search inside video content

  Scenario: A chunk with speech is transcribed and indexed
    Given the AI Worker runs with the default WHISPER_MODEL "small"
    And chunk 42 of video "V" contains English speech about "data pipelines"
    When its "VideoChunkUploaded" event is delivered to consumer "ai-indexer"
    Then the AI Worker extracts the audio track and transcribes it with "faster-whisper/small/int8"
    And the detected language "en" is recorded in the transcript
    And writes the transcript to "af-media/videos/V/transcripts/000042.json"
    And embeds the text with "intfloat/multilingual-e5-small" using the "passage: " prefix
    And upserts one Qdrant point with id UUIDv5("V:000042") into alias "transcripts" (collection "transcripts_me5s384_v1")
    And the point payload has thumbnail_key "videos/V/thumbs/000042.jpg", language "en" and the chunk's start_ms/end_ms
    And "ChunkTranscriptIndexed" with outcome INDEXED, embedding_model "intfloat/multilingual-e5-small" and qdrant_collection "transcripts_me5s384_v1" is published on "video.chunks.ai.indexed.V"

  Scenario: Chunks without speech produce no vector
    When a chunk with silent audio is indexed
    Then "ChunkTranscriptIndexed" is published with outcome NO_SPEECH and no Qdrant point is created
    When a chunk with no audio track is indexed
    Then "ChunkTranscriptIndexed" is published with outcome NO_AUDIO and no transcript is written

  @load
  Scenario: Cold-path latency target without backlog
    Given the AI Worker runs with the default WHISPER_MODEL "small" (int8, CPU)
    And consumer "ai-indexer" has no pending messages
    When 50 speech chunks are published one at a time
    Then p95 latency from publish to "ChunkTranscriptIndexed" is at most 5 seconds
