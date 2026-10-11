# Copied verbatim from affogato_spec.md section 4 (only the feature tag line is added).
# Do not edit here: change the spec, then re-copy. TestFeaturesMatchSpec enforces this.
@retention
Feature: Retention configuration
  As an operator
  I want retention limits applied exactly as configured
  So that storage stays bounded and dead letters can always be redriven

  Scenario: Streams and buckets carry the configured retention
    When the operator inspects the deployed NATS and MinIO configuration
    Then streams "VIDEO_EVENTS", "VIDEO_DLQ" and "AF_ADVISORIES" each have max_age 7 days
    And bucket "af-raw" expires objects after 10 days
    And bucket "af-media" has no expiration rule
    And the "af-raw" expiry is longer than the "VIDEO_DLQ" max_age
