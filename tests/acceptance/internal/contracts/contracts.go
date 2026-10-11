// Package contracts holds test-side builders for the FramePulse wire contracts
// (docs/architecture-review.md Part B): subjects, Nats-Msg-Id values, object keys,
// HLS_INDEX keys, Serving API paths and Qdrant point IDs.
//
// These builders exist so acceptance tests can assert exact names without depending on
// production code. Once libs/go/fpcontracts and contracts/testdata/keys.json exist,
// contracts_test.go should additionally check these builders against keys.json
// (B4 CI step 5) so the test side and the production side cannot drift apart.
package contracts

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Stream, consumer and KV names (B1).
const (
	StreamEvents     = "VIDEO_EVENTS"
	StreamDLQ        = "VIDEO_DLQ"
	StreamAdvisories = "FP_ADVISORIES"
	KVHlsIndex       = "HLS_INDEX"

	ConsumerHls = "hls-stream-worker"
	ConsumerAI  = "ai-indexer"

	SubjectEmbed = "svc.embed.v1.embed"

	RenditionSrc = "src"

	BucketRaw   = "fp-raw"
	BucketMedia = "fp-media"

	QdrantAlias      = "transcripts"
	QdrantCollection = "transcripts_me5s384_v1"
	EmbeddingModel   = "intfloat/multilingual-e5-small"
	EmbeddingDim     = 384
)

// Header names and values (B1 "Headers").
const (
	HeaderMsgID       = "Nats-Msg-Id"
	HeaderContentType = "Content-Type"
	HeaderSchema      = "Fp-Schema"
	HeaderProducer    = "Fp-Producer"
	ContentTypeProto  = "application/protobuf"

	// HeaderTestFault requests a test-only fault in a worker (spec 2.5, review B1
	// "Test-only fault hooks"). Honored only when the worker runs with FP_TEST_FAULTS=1
	// (kind). Workers never copy it onto result events; DeadLetter.original_headers keeps
	// it verbatim; fpctl dlq redrive strips it.
	HeaderTestFault = "Fp-Test-Fault"
)

// Fully qualified protobuf message names, used in the Fp-Schema header.
const (
	SchemaVideoChunkUploaded     = "framepulse.events.v1.VideoChunkUploaded"
	SchemaChunkHlsReady          = "framepulse.events.v1.ChunkHlsReady"
	SchemaChunkTranscriptIndexed = "framepulse.events.v1.ChunkTranscriptIndexed"
	SchemaVideoIngestCompleted   = "framepulse.events.v1.VideoIngestCompleted"
	SchemaDeadLetter             = "framepulse.dlq.v1.DeadLetter"
)

// MaxChunkSeq is the largest chunk_seq representable by the 6-digit seq6 format. Key
// builders reject anything above it (B2). It is a format limit, not the video cap:
// see MaxVideoChunks.
const MaxChunkSeq = 999_999

// Video length cap (spec A13, review decision 14): at most 4 h of media and at most
// 8,000 chunks (chunk_seq 0..7999). Workers dead-letter chunk_seq >= 8000 or
// chunk_count > 8000 as ERROR_CLASS_SCHEMA; the Producer rejects longer files before
// any upload.
const (
	MaxVideoChunks   = 8_000
	MaxVideoDuration = 4 * time.Hour
)

// ValidateChunkSeq reports whether seq is inside the per-video cap (0..7999).
func ValidateChunkSeq(seq int64) error {
	if seq < 0 || seq >= MaxVideoChunks {
		return fmt.Errorf("chunk_seq %d outside 0..%d (spec A13: at most %d chunks per video)", seq, MaxVideoChunks-1, MaxVideoChunks)
	}
	return nil
}

// ValidateChunkCount reports whether a VideoIngestCompleted.chunk_count is valid (1..8000).
func ValidateChunkCount(n int64) error {
	if n < 1 || n > MaxVideoChunks {
		return fmt.Errorf("chunk_count %d outside 1..%d (spec A13)", n, MaxVideoChunks)
	}
	return nil
}

// Seq6 formats a 0-based chunk_seq as the zero-padded 6-digit form used in keys and
// Msg-Ids. It panics above MaxChunkSeq: the contract does not define a 7-digit form,
// so a larger value is a test bug, not something to format silently.
func Seq6(seq uint32) string {
	if seq > MaxChunkSeq {
		panic(fmt.Sprintf("contracts: chunk_seq %d exceeds seq6 range (max %d)", seq, MaxChunkSeq))
	}
	return fmt.Sprintf("%06d", seq)
}

// ParseSeq6 parses a strict 6-digit seq6 token ("000042" -> 42).
func ParseSeq6(s string) (uint32, error) {
	if len(s) != 6 {
		return 0, fmt.Errorf("seq6 %q: want exactly 6 digits", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("seq6 %q: non-digit %q", s, r)
		}
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}

// ---- Subjects (last token is always video_id) ----

func SubjectChunkUploaded(videoID string) string { return "video.chunks.uploaded." + videoID }
func SubjectHlsReady(videoID string) string      { return "video.chunks.hls.ready." + videoID }
func SubjectAIIndexed(videoID string) string     { return "video.chunks.ai.indexed." + videoID }
func SubjectIngestCompleted(videoID string) string {
	return "video.lifecycle.completed." + videoID
}

// SubjectDLQ is dlq.{consumer}.{video_id}; pass "unknown" when the payload was undecodable.
func SubjectDLQ(consumer, videoID string) string { return "dlq." + consumer + "." + videoID }

// EventFilterSubjects returns the VIDEO_EVENTS filter subjects that select every event of
// one video (review B1 "Per-video filters", spec section 4). NATS '*' matches exactly one
// token, so a single "video.chunks.*.{id}" filter would only match the 4-token "uploaded"
// subject and miss the 5-token "hls.ready" and "ai.indexed" subjects; both shapes are
// needed. Together with DLQFilterSubjects these are the four documented filters.
func EventFilterSubjects(videoID string) []string {
	return []string{
		"video.chunks.*." + videoID,   // video.chunks.uploaded.{id}
		"video.chunks.*.*." + videoID, // video.chunks.hls.ready.{id}, video.chunks.ai.indexed.{id}
		"video.lifecycle.*." + videoID,
	}
}

// DLQFilterSubjects returns the VIDEO_DLQ filter subjects for one video (all consumers).
// Dead letters with an undecodable payload go to dlq.{consumer}.unknown and are not matched.
func DLQFilterSubjects(videoID string) []string { return []string{"dlq.*." + videoID} }

// ---- Test-only fault hooks (spec 2.5) ----

// TestFault is a fault name for the Fp-Test-Fault header.
type TestFault string

const (
	FaultExitOnReceive  TestFault = "exit-on-receive"  // exit 70 before processing; default n unlimited
	FaultCrashBeforeAck TestFault = "crash-before-ack" // process, PubAck, then exit 70 instead of ack; default n 1
	FaultFailTransient  TestFault = "fail-transient"   // transient error before any side effect; default n unlimited
)

func (f TestFault) valid() bool {
	return f == FaultExitOnReceive || f == FaultCrashBeforeAck || f == FaultFailTransient
}

func validDurable(c string) bool { return c == ConsumerHls || c == ConsumerAI }

// TestFaultValue builds an Fp-Test-Fault header value:
// "<fault>; consumer=<durable>[; deliveries=<n>]". deliveries 0 omits the parameter (the
// fault's default n applies); negative values are rejected.
func TestFaultValue(fault TestFault, consumer string, deliveries int) (string, error) {
	if !fault.valid() {
		return "", fmt.Errorf("unknown test fault %q", fault)
	}
	if !validDurable(consumer) {
		return "", fmt.Errorf("test fault consumer %q: want %s or %s", consumer, ConsumerHls, ConsumerAI)
	}
	if deliveries < 0 {
		return "", fmt.Errorf("test fault deliveries %d: want a positive integer", deliveries)
	}
	v := string(fault) + "; consumer=" + consumer
	if deliveries > 0 {
		v += "; deliveries=" + strconv.Itoa(deliveries)
	}
	return v, nil
}

// ParsedTestFault is a decoded Fp-Test-Fault value. Deliveries is 0 when not given.
type ParsedTestFault struct {
	Fault      TestFault
	Consumer   string
	Deliveries int
}

// ParseTestFaultValue applies the header grammar of review B1 strictly, as workers must
// (malformed values are dead-lettered as TEST_FAULT_INVALID, never ignored).
func ParseTestFaultValue(v string) (ParsedTestFault, error) {
	parts := strings.Split(v, ";")
	var p ParsedTestFault
	p.Fault = TestFault(strings.TrimSpace(parts[0]))
	if !p.Fault.valid() {
		return p, fmt.Errorf("test fault %q: unknown fault %q", v, p.Fault)
	}
	seen := map[string]bool{}
	for _, kv := range parts[1:] {
		k, val, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok || seen[k] {
			return p, fmt.Errorf("test fault %q: bad or repeated parameter %q", v, kv)
		}
		seen[k] = true
		switch k {
		case "consumer":
			if !validDurable(val) {
				return p, fmt.Errorf("test fault %q: unknown consumer %q", v, val)
			}
			p.Consumer = val
		case "deliveries":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || val != strconv.Itoa(n) {
				return p, fmt.Errorf("test fault %q: deliveries %q is not a positive integer", v, val)
			}
			p.Deliveries = n
		default:
			return p, fmt.Errorf("test fault %q: unknown parameter %q", v, k)
		}
	}
	if p.Consumer == "" {
		return p, fmt.Errorf("test fault %q: missing consumer", v)
	}
	return p, nil
}

// ---- Nats-Msg-Id values ----

func MsgIDUploaded(videoID string, seq uint32) string {
	return "uploaded:" + videoID + ":" + Seq6(seq)
}
func MsgIDHlsReady(videoID, rendition string, seq uint32) string {
	return "hls-ready:" + videoID + ":" + rendition + ":" + Seq6(seq)
}
func MsgIDAIIndexed(videoID string, seq uint32) string {
	return "ai-indexed:" + videoID + ":" + Seq6(seq)
}
func MsgIDCompleted(videoID string) string { return "completed:" + videoID }
func MsgIDDLQ(consumer string, originalStreamSeq uint64) string {
	return "dlq:" + consumer + ":" + strconv.FormatUint(originalStreamSeq, 10)
}

// MsgIDRedrive is the Msg-Id fpctl uses when republishing a dead letter: {orig}:redrive:{n}.
func MsgIDRedrive(originalMsgID string, n int) string {
	return originalMsgID + ":redrive:" + strconv.Itoa(n)
}

// ---- Object locations ----

// Object is a bucket + key pair. Spec text writes them joined ("fp-raw/videos/…").
type Object struct {
	Bucket string
	Key    string
}

func (o Object) String() string { return o.Bucket + "/" + o.Key }

// ParseObject splits "bucket/key" as written in the spec. Only the known FramePulse
// buckets are accepted, and keys containing "..", "//", backslashes or a leading "/"
// are rejected so step text can never address objects outside the test video.
func ParseObject(s string) (Object, error) {
	bucket, key, ok := strings.Cut(s, "/")
	if !ok || key == "" {
		return Object{}, fmt.Errorf("object %q: want bucket/key", s)
	}
	if bucket != BucketRaw && bucket != BucketMedia {
		return Object{}, fmt.Errorf("object %q: unknown bucket %q", s, bucket)
	}
	if err := ValidateKey(key); err != nil {
		return Object{}, fmt.Errorf("object %q: %w", s, err)
	}
	return Object{Bucket: bucket, Key: key}, nil
}

// ValidateKey rejects object keys that could escape a video prefix.
func ValidateKey(key string) error {
	switch {
	case key == "":
		return fmt.Errorf("empty key")
	case strings.HasPrefix(key, "/"):
		return fmt.Errorf("key %q: leading slash", key)
	case strings.Contains(key, "\\"):
		return fmt.Errorf("key %q: backslash", key)
	case strings.Contains(key, "//"):
		return fmt.Errorf("key %q: empty path segment", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("key %q: dot segment", key)
		}
	}
	return nil
}

func videoPrefix(videoID string) string { return "videos/" + videoID + "/" }

func RawChunk(videoID string, seq uint32) Object {
	return Object{BucketRaw, videoPrefix(videoID) + "chunks/" + Seq6(seq) + ".ts"}
}
func HlsSegment(videoID, rendition string, seq uint32) Object {
	return Object{BucketMedia, videoPrefix(videoID) + "hls/" + rendition + "/" + Seq6(seq) + ".ts"}
}
func Thumbnail(videoID string, seq uint32) Object {
	return Object{BucketMedia, videoPrefix(videoID) + "thumbs/" + Seq6(seq) + ".jpg"}
}
func Transcript(videoID string, seq uint32) Object {
	return Object{BucketMedia, videoPrefix(videoID) + "transcripts/" + Seq6(seq) + ".json"}
}
func MediaPlaylist(videoID, rendition string) Object {
	return Object{BucketMedia, videoPrefix(videoID) + "hls/" + rendition + "/playlist.m3u8"}
}
func MasterPlaylist(videoID string) Object {
	return Object{BucketMedia, videoPrefix(videoID) + "hls/master.m3u8"}
}

// VideoPrefix returns the per-video prefix in a bucket, used for cleanup listings.
func VideoPrefix(bucket, videoID string) Object { return Object{bucket, videoPrefix(videoID)} }

// Content types per artifact (B2 key table).
const (
	ContentTypeTS       = "video/mp2t"
	ContentTypeM3U8     = "application/vnd.apple.mpegurl"
	ContentTypeJPEG     = "image/jpeg"
	ContentTypeJSON     = "application/json"
	CacheControlSegment = "public, max-age=31536000, immutable"
	CacheControlList    = "max-age=86400"
)

// ---- HLS_INDEX ----

func HlsIndexKey(videoID, rendition string) string { return videoID + "." + rendition }

// ---- Serving API paths ----

const APIPrefix = "/api/v1"

func APIMasterPlaylist(videoID string) string {
	return APIPrefix + "/videos/" + url.PathEscape(videoID) + "/hls/master.m3u8"
}
func APIMediaPlaylist(videoID, rendition string) string {
	return APIPrefix + "/videos/" + url.PathEscape(videoID) + "/hls/" + url.PathEscape(rendition) + "/playlist.m3u8"
}
func APISegment(videoID, rendition string, seq uint32) string {
	return APIPrefix + "/videos/" + url.PathEscape(videoID) + "/hls/" + url.PathEscape(rendition) + "/" + Seq6(seq) + ".ts"
}
func APIThumbnail(videoID string, seq uint32) string {
	return APIPrefix + "/videos/" + url.PathEscape(videoID) + "/thumbs/" + Seq6(seq) + ".jpg"
}

// APISearch builds /api/v1/search?q=…&limit=…; limit <= 0 omits the parameter.
func APISearch(q string, limit int) string { return searchPath(APIPrefix+"/search", q, limit) }

// APISearchLegacy builds the /api/search alias.
func APISearchLegacy(q string, limit int) string { return searchPath("/api/search", q, limit) }

func searchPath(base, q string, limit int) string {
	v := url.Values{}
	v.Set("q", q)
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	return base + "?" + v.Encode()
}

// ---- IDs ----

var videoIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidateVideoID enforces the contract format: lowercase canonical UUIDv7, 36 chars,
// RFC 9562 variant.
func ValidateVideoID(s string) error {
	if !videoIDRe.MatchString(s) {
		return fmt.Errorf("video_id %q is not a lowercase canonical UUIDv7", s)
	}
	return nil
}

// NewVideoID returns a fresh lowercase UUIDv7.
func NewVideoID() (string, error) {
	u, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PointIDNamespace is the fixed UUIDv5 namespace for Qdrant point IDs (spec 2.2 and A18,
// review B3). It never changes: a new namespace changes every point ID (full re-index).
var PointIDNamespace = uuid.MustParse("c1907ffe-0c06-4077-8866-08fb8d658b77")

// PointID returns UUIDv5(PointIDNamespace, "{video_id}:{seq6}").
func PointID(videoID string, seq uint32) uuid.UUID {
	return uuid.NewSHA1(PointIDNamespace, []byte(videoID+":"+Seq6(seq)))
}

// PointIDFromName returns UUIDv5(PointIDNamespace, name) for names already in
// "{video_id}:{seq6}" form, as written in the feature files (UUIDv5("V:000042")).
func PointIDFromName(name string) (uuid.UUID, error) {
	vid, seq, ok := strings.Cut(name, ":")
	if !ok {
		return uuid.Nil, fmt.Errorf("point name %q: want {video_id}:{seq6}", name)
	}
	if err := ValidateVideoID(vid); err != nil {
		return uuid.Nil, err
	}
	if _, err := ParseSeq6(seq); err != nil {
		return uuid.Nil, err
	}
	return uuid.NewSHA1(PointIDNamespace, []byte(name)), nil
}

// ThumbnailKeyPayload is the bucket-less thumbnail key stored in the Qdrant payload
// ("videos/V/thumbs/000042.jpg").
func ThumbnailKeyPayload(videoID string, seq uint32) string { return Thumbnail(videoID, seq).Key }
