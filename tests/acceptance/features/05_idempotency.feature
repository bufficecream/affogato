# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@idempotency
Feature: Idempotent processing
  As an operator
  I want redelivered or duplicate events to be harmless
  So that at-least-once delivery never creates duplicate outputs

  Scenario: Duplicate publish is deduplicated by the stream
    When the Producer publishes "VideoChunkUploaded" for chunk 5 of video "V" twice with the same Nats-Msg-Id
    Then "VIDEO_EVENTS" stores exactly one message for that chunk

  Scenario Outline: Redelivery after a crash before ack produces no duplicates
    Given chunk 5 of video "V" has been fully processed by <worker> but not acked
    When the event is redelivered to <worker>
    Then the same object keys are overwritten with equivalent content
    And <state_check>
    And the result event is deduplicated, so "VIDEO_EVENTS" holds exactly one <result_event> for chunk 5

    Examples:
      | worker        | state_check                                                | result_event           |
      | Stream Worker | "HLS_INDEX" key "V.src" lists chunk 5 exactly once          | ChunkHlsReady          |
      | AI Worker     | Qdrant holds exactly one point with id UUIDv5("V:000005")   | ChunkTranscriptIndexed |
