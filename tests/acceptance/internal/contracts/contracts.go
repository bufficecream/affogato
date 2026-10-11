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
)

// Fully qualified protobuf message names, used in the Fp-Schema header.
const (
	SchemaVideoChunkUploaded     = "framepulse.events.v1.VideoChunkUploaded"
	SchemaChunkHlsReady          = "framepulse.events.v1.ChunkHlsReady"
	SchemaChunkTranscriptIndexed = "framepulse.events.v1.ChunkTranscriptIndexed"
	SchemaVideoIngestCompleted   = "framepulse.events.v1.VideoIngestCompleted"
	SchemaDeadLetter             = "framepulse.dlq.v1.DeadLetter"
)

// MaxChunkSeq is the largest chunk_seq representable by the 6-digit seq6 format.
// At ~2 s per chunk this is ~23 days, well above the 24 h session limit (spec A13).
const MaxChunkSeq = 999_999

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
// one video. NATS '*' matches exactly one token, so a single "video.chunks.*.{id}" filter
// would only match the 4-token "uploaded" subject and miss the 5-token "hls.ready" and
// "ai.indexed" subjects; both shapes are needed.
func EventFilterSubjects(videoID string) []string {
	return []string{
		"video.chunks.*." + videoID,   // video.chunks.uploaded.{id}
		"video.chunks.*.*." + videoID, // video.chunks.hls.ready.{id}, video.chunks.ai.indexed.{id}
		"video.lifecycle.*." + videoID,
	}
}

// DLQFilterSubjects returns the VIDEO_DLQ filter subjects for one video (all consumers).
func DLQFilterSubjects(videoID string) []string { return []string{"dlq.*." + videoID} }

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

// PointIDNamespacePlaceholder is the UUIDv5 namespace for Qdrant point IDs.
//
// TODO(contracts): B3 says "UUIDv5 over {video_id}:{seq6} with a fixed FramePulse
// namespace UUID" but no document defines the UUID. Until contracts/testdata/keys.json
// pins it, this placeholder is used and can be overridden with FP_POINT_ID_NAMESPACE.
// Tests that compare point IDs against the real AI Worker fail until both sides agree.
var PointIDNamespacePlaceholder = uuid.MustParse("00000000-0000-0000-0000-000000000000")

// PointID returns UUIDv5(namespace, "{video_id}:{seq6}").
func PointID(namespace uuid.UUID, videoID string, seq uint32) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(videoID+":"+Seq6(seq)))
}

// PointIDFromName returns UUIDv5(namespace, name) for names already in "{video_id}:{seq6}"
// form, as written in the feature files (UUIDv5("V:000042")).
func PointIDFromName(namespace uuid.UUID, name string) (uuid.UUID, error) {
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
	return uuid.NewSHA1(namespace, []byte(name)), nil
}

// ThumbnailKeyPayload is the bucket-less thumbnail key stored in the Qdrant payload
// ("videos/V/thumbs/000042.jpg").
func ThumbnailKeyPayload(videoID string, seq uint32) string { return Thumbnail(videoID, seq).Key }
