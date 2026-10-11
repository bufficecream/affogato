# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@failures
Feature: Failure handling on both paths
  As an operator
  I want failing events isolated with enough context to fix and replay them
  So that one bad input or outage does not stall or crash the pipeline

  Scenario: AI Worker failure is retried, then dead-lettered, without affecting the hot path
    Given Qdrant is unavailable
    When a "VideoChunkUploaded" event for chunk 7 of video "V" is published
    Then the AI Worker NAKs deliveries 1 to 4 with increasing backoff delays
    And when delivery 5 also fails it publishes a "DeadLetter" on "dlq.ai-indexer.V" with "ERROR_CLASS_RETRIES_EXHAUSTED" and delivery_count 5
    And terminates the message without a 6th delivery
    And the dead letter contains the original subject, headers and payload verbatim
    And the Stream Worker still produces "ChunkHlsReady" for chunk 7

  Scenario: Crash loop is broken by the delivery cap
    Given a message that makes the AI Worker process exit before acking
    When it has been delivered 5 times and the worker exited each time
    Then the worker that receives the 6th delivery sends it to the DLQ before processing, with delivery_count 6, and terminates it

  Scenario: Redriving dead letters after recovery
    Given dead letters from consumer "ai-indexer" exist and Qdrant is available again
    When the operator runs "afctl dlq redrive --consumer ai-indexer --since 1h"
    Then the original payloads are republished to their original subjects
    And each affected chunk is eventually indexed exactly once
