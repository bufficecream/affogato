# FramePulse: System Architecture & BDD Master Specifications

## 1. Project Overview & Glossary Context
**FramePulse** is a cloud-native, distributed video processing and semantic search pipeline designed to explore real-time stream processing, asynchronous AI analysis, and vector-based search using Go and Rust.

### Key Terminology Reference
- **Producer (Go):** Simulates video cameras or client uploads, chopping raw videos into 2-second chunks (`.ts`) and publishing events.
- **Message Queue (NATS JetStream):** Provides decouplings, buffer, and durability for event distribution (`video.chunks`).
- **Stream Processing Worker (Rust):** Consumes raw chunks in the Hot Path to instantly generate HLS playlists (`.m3u8`) and thumbnails.
- **AI Processing Worker (Python/Go):** Consumes events in the Cold Path to extract audio, perform Speech-to-Text (Whisper), and generate vector embeddings.
- **Object Storage (MinIO):** S3-compatible storage for raw chunks, HLS manifests, and thumbnails.
- **Vector Database (Qdrant):** Stores embeddings for semantic search over video transcripts.
- **Serving Layer (Go - Axum/Gin):** Exposes APIs for HLS video playback and semantic text-based video search.

---

## 2. System Architecture Design (Hot Path vs. Cold Path)

```text
[Producer (Go CLI)]
       │
       ▼
[NATS JetStream MQ] (Topic: video.events)
       │
       ├───────────────┬───────────────┐ (Pub/Sub)
       ▼ (Hot Path)                    ▼ (Cold Path)
[Stream Worker (Rust)]          [AI Worker (Python/Go)]
 ├── HLS Transcoding (2s chunks) ├── Whisper Speech-to-Text
 └── Manifest Generation (.m3u8) └── Vector Embedding Generation
       │                               │
       ▼                               ▼
[MinIO Object Storage]          [Qdrant Vector DB]
       │                               │
       └───────────────┬───────────────┘
                       ▼
             [Serving API (Go)] ──> HLS Player & Search UI
```

---

## 3. BDD Specifications (Gherkin Feature Files)

### Feature 1: Real-time Video Ingestion and HLS Generation (Hot Path)
```gherkin
Feature: Real-time Video Chunk Processing
  As a system user
  I want my uploaded video chunks to be immediately transcodeable
  So that I can play the video via HLS without waiting for the full upload

  Scenario: Successful processing of a valid 1080p video chunk
    Given the NATS message queue and Stream Worker pods are running in Kubernetes
    When the Producer CLI uploads a test chunk "sample_01.mp4" to MinIO
    And publishes a "VideoChunkUploaded" event to NATS topic "video.chunks"
    Then the Stream Worker should consume the event within 500 milliseconds
    And successfully generate "playlist.m3u8" and "chunk_01.ts" in MinIO
    And an HTTP GET request to the Serving API for the playlist should return status 200 OK

  Scenario: Handling invalid or corrupted video chunks
    Given the Stream Worker is listening to NATS queue
    When an event with a corrupted file reference is published
    Then the Stream Worker should catch the decoding error
    And route the failed event to the Dead Letter Queue (DLQ)
    And log the incident without crashing the worker pod
```

### Feature 2: Asynchronous AI Transcription and Vector Indexing (Cold Path)
```gherkin
Feature: Asynchronous AI Transcription and Vector Indexing
  As a content consumer
  I want video audio to be automatically transcribed and indexed
  So that I can search inside the video content later

  Scenario: Processing a video chunk through the AI Worker pipeline
    Given the AI Worker and Qdrant vector database are deployed in Kubernetes
    When a "VideoChunkUploaded" event is broadcasted to the AI Worker consumer group
    Then the AI Worker should extract the audio track from the chunk
    And generate a transcript segment using the STT engine
    And store the corresponding vector embedding into Qdrant within 5 seconds
    And verify that a vector search query returns the correct chunk metadata
```

### Feature 3: Semantic Search over Processed Videos (Serving Layer)
```gherkin
Feature: Semantic Search over Processed Videos
  As a viewer
  I want to search for specific topics spoken in the video
  So that I can jump directly to the exact timestamp

  Scenario: Searching for a keyword returns precise video timestamps and keyframes
    Given video chunks have been processed and indexed in Qdrant and MinIO
    When the user sends a GET request to "/api/search?q=architecture" via the Serving API
    Then the API should return a list of matching results containing:
      | Field     | Expected Value                  |
      | timestamp | 00:01:25 - 00:01:40             |
      | snippet   | "...talking about data pipelines"|
      | thumbnail | URL pointing to MinIO image     |
```