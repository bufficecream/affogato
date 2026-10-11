// Package payload decouples step definitions from generated protobuf code.
//
// Steps work with the small "view" structs below, which hold only the fields the
// acceptance criteria assert on. A Codec converts between views and wire bytes.
//
// TODO(contracts): once gen/go exists (B4), add a ProtoCodec in this package that
// imports github.com/affogato/affogato/gen/go/affogato/... (add a `replace`
// directive in go.mod pointing at ../../gen/go), maps the generated messages to these
// views, and becomes the Default. Until then every call returns ErrNotImplemented, and
// steps that need it stay pending.
package payload

import (
	"errors"
	"fmt"
	"time"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

// ErrNotImplemented is returned by PendingCodec. Steps map it to godog.ErrPending.
var ErrNotImplemented = errors.New("protobuf codec not implemented: generated code (gen/go) does not exist yet")

type TimeRange struct{ StartMs, EndMs int64 }

func (r TimeRange) DurationMs() int64 { return r.EndMs - r.StartMs }

type ChunkRef struct {
	VideoID  string
	ChunkSeq uint32
}

type ObjectRef struct {
	Bucket, Key, ContentType, SHA256Hex string
	SizeBytes                           int64
}

// VideoChunkUploaded (affogato.events.v1.VideoChunkUploaded) - tests publish these directly
// in scenarios that bypass the Producer CLI (out-of-order, corrupted chunk, duplicates).
type VideoChunkUploaded struct {
	EventID   string
	Chunk     ChunkRef
	RawObject ObjectRef
	MediaTime TimeRange
	HasAudio  bool
	SourceID  string
}

type VideoIngestCompleted struct {
	EventID    string
	VideoID    string
	ChunkCount uint32
	MediaTime  TimeRange
}

type ChunkHlsReady struct {
	Chunk     ChunkRef
	Rendition string
	Segment   ObjectRef
	Thumbnail ObjectRef
	MediaTime TimeRange
}

type IndexOutcome int

const (
	OutcomeUnspecified IndexOutcome = iota
	OutcomeIndexed
	OutcomeNoSpeech
	OutcomeNoAudio
)

type ChunkTranscriptIndexed struct {
	Chunk            ChunkRef
	MediaTime        TimeRange
	Outcome          IndexOutcome
	Transcript       *ObjectRef // nil when NO_AUDIO
	Language         string
	SttModel         string
	EmbeddingModel   string
	QdrantCollection string
	PointID          string
}

type Header struct {
	Name   string
	Values []string
}

type DeadLetter struct {
	OriginalStream    string
	OriginalStreamSeq uint64
	OriginalSubject   string
	OriginalHeaders   []Header
	OriginalPayload   []byte
	Consumer          string
	DeliveryCount     uint32
	ErrorClass        string // enum name, e.g. "ERROR_CLASS_INVALID_MEDIA"
	ErrorCode         string
	Redrivable        bool
	Chunk             *ChunkRef
}

type SegmentStatus int

const (
	SegmentUnspecified SegmentStatus = iota
	SegmentReady
	SegmentFailed
)

// Segment mirrors affogato.state.v1.Segment. There is no key field: Segment.key was
// dropped (field 2 reserved, review decision 14); the segment key is derived from
// chunk_seq with contracts.HlsSegment.
type Segment struct {
	ChunkSeq  uint32
	MediaTime TimeRange
	Status    SegmentStatus
}

// MaxHlsIndexBytes is the encoded-size budget the acceptance run checks HLS_INDEX values
// against: review B1 bounds an 8,000-segment index at ~152 KB (+ header fields); 160 KB
// leaves a little slack while staying far below the 1 MiB max_value_size.
const MaxHlsIndexBytes = 160_000

type HlsIndex struct {
	VideoID            string
	Rendition          string
	Segments           []Segment
	ExpectedChunkCount uint32 // 0 = unknown
	Finalized          bool
	LastActivityAt     time.Time
	FinalizeReason     string // "FINALIZE_REASON_COMPLETED" / "FINALIZE_REASON_IDLE_TIMEOUT"
}

// Transcript is stored in MinIO as canonical proto3 JSON (B1 supporting contracts).
type Transcript struct {
	Chunk     ChunkRef
	MediaTime TimeRange
	Language  string
	SttModel  string
	Text      string
}

// Codec encodes events the tests publish and decodes everything they assert on.
type Codec interface {
	EncodeVideoChunkUploaded(VideoChunkUploaded) ([]byte, error)
	EncodeVideoIngestCompleted(VideoIngestCompleted) ([]byte, error)

	DecodeVideoChunkUploaded([]byte) (VideoChunkUploaded, error)
	DecodeChunkHlsReady([]byte) (ChunkHlsReady, error)
	DecodeChunkTranscriptIndexed([]byte) (ChunkTranscriptIndexed, error)
	DecodeDeadLetter([]byte) (DeadLetter, error)
	DecodeHlsIndex([]byte) (HlsIndex, error)
	DecodeTranscriptJSON([]byte) (Transcript, error)
}

// PendingCodec is the placeholder Codec until generated code exists.
type PendingCodec struct{}

func (PendingCodec) EncodeVideoChunkUploaded(VideoChunkUploaded) ([]byte, error) {
	return nil, ErrNotImplemented
}
func (PendingCodec) EncodeVideoIngestCompleted(VideoIngestCompleted) ([]byte, error) {
	return nil, ErrNotImplemented
}
func (PendingCodec) DecodeVideoChunkUploaded([]byte) (VideoChunkUploaded, error) {
	return VideoChunkUploaded{}, ErrNotImplemented
}
func (PendingCodec) DecodeChunkHlsReady([]byte) (ChunkHlsReady, error) {
	return ChunkHlsReady{}, ErrNotImplemented
}
func (PendingCodec) DecodeChunkTranscriptIndexed([]byte) (ChunkTranscriptIndexed, error) {
	return ChunkTranscriptIndexed{}, ErrNotImplemented
}
func (PendingCodec) DecodeDeadLetter([]byte) (DeadLetter, error) {
	return DeadLetter{}, ErrNotImplemented
}
func (PendingCodec) DecodeHlsIndex([]byte) (HlsIndex, error) { return HlsIndex{}, ErrNotImplemented }
func (PendingCodec) DecodeTranscriptJSON([]byte) (Transcript, error) {
	return Transcript{}, ErrNotImplemented
}

// Default is the codec the suite uses. Replace with the generated-code codec (TODO above).
var Default Codec = PendingCodec{}

// ---- Pure helpers over views (unit-tested; no codec needed) ----

// CheckCap verifies the per-video cap (spec A13): at most 8,000 segments, every chunk_seq
// below 8,000 and expected_chunk_count (if known) at most 8,000.
func (x HlsIndex) CheckCap() error {
	if len(x.Segments) > contracts.MaxVideoChunks {
		return fmt.Errorf("HlsIndex %s.%s has %d segments, cap %d", x.VideoID, x.Rendition, len(x.Segments), contracts.MaxVideoChunks)
	}
	for _, s := range x.Segments {
		if err := contracts.ValidateChunkSeq(int64(s.ChunkSeq)); err != nil {
			return fmt.Errorf("HlsIndex %s.%s: %w", x.VideoID, x.Rendition, err)
		}
	}
	if x.ExpectedChunkCount != 0 {
		if err := contracts.ValidateChunkCount(int64(x.ExpectedChunkCount)); err != nil {
			return fmt.Errorf("HlsIndex %s.%s: %w", x.VideoID, x.Rendition, err)
		}
	}
	return nil
}

// CheckEncodedSize checks a raw HLS_INDEX value against MaxHlsIndexBytes.
func CheckEncodedSize(value []byte) error {
	if len(value) > MaxHlsIndexBytes {
		return fmt.Errorf("HLS_INDEX value is %d bytes, budget %d (review B1 KV size budget)", len(value), MaxHlsIndexBytes)
	}
	return nil
}

// SegmentsInOrder reports whether segments are sorted by chunk_seq with no duplicates.
func (x HlsIndex) SegmentsInOrder() bool {
	for i := 1; i < len(x.Segments); i++ {
		if x.Segments[i].ChunkSeq <= x.Segments[i-1].ChunkSeq {
			return false
		}
	}
	return true
}

// CountSeq returns how many index entries exist for seq (must be exactly 1 after redelivery).
func (x HlsIndex) CountSeq(seq uint32) int {
	n := 0
	for _, s := range x.Segments {
		if s.ChunkSeq == seq {
			n++
		}
	}
	return n
}

// Segment returns the entry for seq.
func (x HlsIndex) Segment(seq uint32) (Segment, bool) {
	for _, s := range x.Segments {
		if s.ChunkSeq == seq {
			return s, true
		}
	}
	return Segment{}, false
}

// Counts returns (ready, failed) as reported by the 409 status body.
func (x HlsIndex) Counts() (ready, failed int) {
	for _, s := range x.Segments {
		switch s.Status {
		case SegmentReady:
			ready++
		case SegmentFailed:
			failed++
		}
	}
	return ready, failed
}
