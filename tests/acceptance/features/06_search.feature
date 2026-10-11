# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@search
Feature: Semantic search over processed videos
  As a viewer
  I want to search for topics spoken in videos
  So that I can jump directly to the right moment

  Background:
    Given chunks 42 to 49 of video "V" are indexed, spanning media time 85.000 s to 100.000 s
    And their transcripts discuss "data pipeline architecture"
    And their thumbnails exist in MinIO

  Scenario: Search returns merged time ranges with snippet and thumbnail
    When the user sends GET "/api/v1/search?q=architecture"
    Then the Serving API embeds the query via "svc.embed.v1.embed" with purpose QUERY
    And returns 200 with a result for video "V" containing:
      | Field         | Expected Value                                             |
      | start_ms      | 85000                                                      |
      | end_ms        | 100000                                                     |
      | timestamp     | 00:01:25 - 00:01:40                                        |
      | snippet       | contains "data pipeline"                                   |
      | thumbnail_url | /api/v1/videos/V/thumbs/{seq6}.jpg for a chunk in 42..49   |
      | playback_url  | /api/v1/videos/V/hls/master.m3u8 with a start-time hint    |
      | status        | ready                                                      |
    And adjacent matching chunks of the same video appear as one range, not 8 results

  Scenario: Search shows videos that are still processing, with their status
    Given chunks of video "W" are indexed but video "W" is not finalized yet
    When the user sends GET "/api/v1/search?q=architecture"
    Then the result for video "W" is included with status "processing"
    And its playback_url responds 409 until the video is finalized

  Scenario: Thumbnail URL redirects to a presigned MinIO URL
    When the client requests the thumbnail_url of the result
    Then the API responds 302 with a Location on the public MinIO endpoint
    And the presigned URL expires within 10 minutes

  Scenario: Missing thumbnail falls back to a placeholder
    Given the thumbnail for the best-matching chunk does not exist yet
    When the client requests its thumbnail_url
    Then the API responds with a placeholder image instead of an error

  Scenario: Legacy search path remains available
    When the user sends GET "/api/search?q=architecture"
    Then the response is identical to GET "/api/v1/search?q=architecture"
