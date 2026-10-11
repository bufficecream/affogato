# Copied verbatim from framepulse_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@finalization
Feature: Video finalization
  As a viewer
  I want a finished upload to become a complete, closed HLS stream
  So that players treat it as on-demand video

  Scenario: Playlist requests before finalization report processing status
    Given chunks 0 and 1 of video "V" are READY in "HLS_INDEX" and no "VideoIngestCompleted" has been processed
    When the client sends GET "/api/v1/videos/V/hls/src/playlist.m3u8"
    Then the API responds 409 with Content-Type "application/json" and "Cache-Control: no-store"
    And the body has status "processing", segments_ready 2, segments_failed 0 and expected_chunk_count null
    And GET "/api/v1/videos/V/hls/master.m3u8" responds the same way
    And GET "/api/v1/videos/{an unknown UUIDv7}/hls/master.m3u8" responds 404

  Scenario: VideoIngestCompleted finalizes the video
    Given chunks 0 to 4 of video "V" are READY in "HLS_INDEX"
    When the Producer publishes "VideoIngestCompleted" with chunk_count 5 on "video.lifecycle.completed.V"
    Then the Stream Worker writes "fp-media/videos/V/hls/src/playlist.m3u8" with 5 segments, "#EXT-X-PLAYLIST-TYPE:VOD" and "#EXT-X-ENDLIST"
    And writes "fp-media/videos/V/hls/master.m3u8" with exactly one variant "src/playlist.m3u8"
    And only after both objects exist is "HLS_INDEX" key "V.src" marked finalized
    And GET "/api/v1/videos/V/hls/src/playlist.m3u8" returns 200 with the content of that object, not a redirect
    And GET "/api/v1/videos/V/hls/master.m3u8" returns 200 and its variant URI resolves to "/api/v1/videos/V/hls/src/playlist.m3u8"

  Scenario: Completion arrives before the last chunk is processed
    Given chunks 0 to 3 of video "V" are READY and chunk 4 is still being processed
    When "VideoIngestCompleted" with chunk_count 5 is delivered
    Then no static playlist is written yet
    And GET "/api/v1/videos/V/hls/src/playlist.m3u8" responds 409 with expected_chunk_count 5
    When chunk 4 becomes READY
    Then the static playlist is written once, with 5 segments and "#EXT-X-ENDLIST"
    And GET "/api/v1/videos/V/hls/src/playlist.m3u8" returns 200

  Scenario: An abandoned upload is force-finalized after the idle timeout
    Given the Stream Worker runs with FINALIZE_IDLE_TIMEOUT "5m"
    And chunks 0, 1 and 3 of video "V" are READY and no "VideoIngestCompleted" has been published
    When no event for video "V" arrives for 5 minutes
    Then the video is finalized with finalize_reason IDLE_TIMEOUT
    And the static playlist lists "000000.ts", "000001.ts" and "000003.ts" with "#EXT-X-DISCONTINUITY" before "000003.ts"
    And the playlist ends with "#EXT-X-ENDLIST"
    And GET "/api/v1/videos/V/hls/src/playlist.m3u8" returns 200

  Scenario: An abandoned upload with no playable chunk is reported as failed
    Given chunk 0 of video "V" is FAILED, no other chunk exists and the idle timeout has passed
    Then the video is finalized with finalize_reason IDLE_TIMEOUT and no playlist object is written
    And GET "/api/v1/videos/V/hls/src/playlist.m3u8" responds 410 with status "failed"

  Scenario: A chunk arriving after finalization does not reopen the video
    Given video "V" was force-finalized after the idle timeout
    When a "VideoChunkUploaded" event for a new chunk of video "V" is delivered
    Then the static playlist is unchanged and "HLS_INDEX" key "V.src" stays finalized
    And metric "fp_late_chunks_total" is incremented
