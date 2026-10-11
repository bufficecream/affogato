# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@hot-path
Feature: Chunk processing into HLS segments
  As a viewer
  I want uploaded video to be turned into HLS segments within seconds
  So that it is playable on demand shortly after the upload finishes

  Background:
    Given the Affogato Helm chart is deployed to the kind test cluster and all pods are ready
    And a new video_id "V" has been assigned by the Producer CLI

  Scenario: A valid video becomes HLS segments and plays as VOD after finalization
    When the Producer CLI ingests "sample_1080p.mp4" (H.264 + AAC) as video "V"
    Then the raw object "af-raw/videos/V/chunks/000000.ts" exists with content-type "video/mp2t"
    And a "VideoChunkUploaded" event for chunk 0 is published on "video.chunks.uploaded.V"
    And the Stream Worker writes "af-media/videos/V/hls/src/000000.ts" by remuxing, without re-encoding
    And writes the thumbnail "af-media/videos/V/thumbs/000000.jpg"
    And "HLS_INDEX" key "V.src" lists chunk 0 as READY with its measured duration
    And a "ChunkHlsReady" event for chunk 0 is published on "video.chunks.hls.ready.V"
    When "HLS_INDEX" key "V.src" is marked finalized
    Then GET "/api/v1/videos/V/hls/src/playlist.m3u8" returns 200 with "#EXT-X-PLAYLIST-TYPE:VOD" and "#EXT-X-ENDLIST"
    And the response body is byte-identical to the object "af-media/videos/V/hls/src/playlist.m3u8"
    And the playlist references "000000.ts" with an EXTINF equal to the measured duration
    And GET "/api/v1/videos/V/hls/src/000000.ts" returns 302 to a presigned MinIO URL

  @load
  Scenario: Hot-path latency targets under nominal load
    When the Producer CLI publishes 300 chunks at real-time rate (one per 2 s per video, 5 videos)
    Then p99 publish-to-delivery latency for consumer "hls-stream-worker" is at most 500 ms
    And p95 publish-to-ready latency (segment READY in HLS_INDEX) is at most 1.5 s

  Scenario: Out-of-order chunks still produce an ordered playlist
    When chunks 2, 0 and 1 of video "V" are processed in that order by different replicas
    And "VideoIngestCompleted" with chunk_count 3 is delivered
    Then "HLS_INDEX" key "V.src" lists chunks 0, 1, 2 in order exactly once each
    And the static playlist "af-media/videos/V/hls/src/playlist.m3u8" lists "000000.ts", "000001.ts", "000002.ts" in order

  Scenario: A corrupted chunk is dead-lettered without crashing the worker
    Given video "V" has 6 chunks and the raw chunk object for chunk 3 contains invalid media bytes
    When the "VideoChunkUploaded" events for chunks 0 to 5 and "VideoIngestCompleted" with chunk_count 6 are published
    Then the Stream Worker's ffprobe process fails on chunk 3 and is classified as "ERROR_CLASS_INVALID_MEDIA"
    And a "DeadLetter" is published on "dlq.hls-stream-worker.V" with redrivable = false
    And the original message is terminated, not redelivered
    And chunk 3 is recorded as FAILED in "HLS_INDEX"
    And the Stream Worker remains healthy and processes chunks 4 and 5 normally
    And the video is finalized with a static playlist that omits "000003.ts"
    And that playlist has "#EXT-X-DISCONTINUITY" immediately before "000004.ts"
