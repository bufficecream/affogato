package contracts

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

const vid = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f" // example from architecture-review.md

func TestSeq6(t *testing.T) {
	cases := map[uint32]string{0: "000000", 5: "000005", 42: "000042", 123456: "123456", MaxChunkSeq: "999999"}
	for in, want := range cases {
		if got := Seq6(in); got != want {
			t.Errorf("Seq6(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSeq6PanicsOutOfRange(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Seq6(1_000_000) did not panic")
		}
	}()
	_ = Seq6(MaxChunkSeq + 1)
}

func TestParseSeq6(t *testing.T) {
	if n, err := ParseSeq6("000042"); err != nil || n != 42 {
		t.Fatalf("ParseSeq6(000042) = %d, %v", n, err)
	}
	for _, bad := range []string{"42", "0000042", "00004a", "-00001", "", "+00001"} {
		if _, err := ParseSeq6(bad); err == nil {
			t.Errorf("ParseSeq6(%q) accepted", bad)
		}
	}
}

func TestSubjects(t *testing.T) {
	cases := []struct{ got, want string }{
		{SubjectChunkUploaded(vid), "video.chunks.uploaded." + vid},
		{SubjectHlsReady(vid), "video.chunks.hls.ready." + vid},
		{SubjectAIIndexed(vid), "video.chunks.ai.indexed." + vid},
		{SubjectIngestCompleted(vid), "video.lifecycle.completed." + vid},
		{SubjectDLQ(ConsumerHls, vid), "dlq.hls-stream-worker." + vid},
		{SubjectDLQ(ConsumerAI, "unknown"), "dlq.ai-indexer.unknown"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

// subjectMatches implements NATS wildcard semantics ('*' = one token, '>' = tail).
func subjectMatches(filter, subject string) bool {
	f, s := strings.Split(filter, "."), strings.Split(subject, ".")
	for i, tok := range f {
		if tok == ">" {
			return len(s) > i
		}
		if i >= len(s) || (tok != "*" && tok != s[i]) {
			return false
		}
	}
	return len(f) == len(s)
}

// The filters must select every per-video event subject and nothing of another video.
func TestEventFilterSubjectsCoverAllEventSubjects(t *testing.T) {
	other := "0192f3a4-7b1c-7d2e-9f00-ffffffffffff"
	matchAny := func(subject string, filters []string) bool {
		for _, f := range filters {
			if subjectMatches(f, subject) {
				return true
			}
		}
		return false
	}
	filters := EventFilterSubjects(vid)
	for _, s := range []string{SubjectChunkUploaded(vid), SubjectHlsReady(vid), SubjectAIIndexed(vid), SubjectIngestCompleted(vid)} {
		if !matchAny(s, filters) {
			t.Errorf("filters %v miss %q", filters, s)
		}
		if matchAny(strings.Replace(s, vid, other, 1), filters) {
			t.Errorf("filters %v match another video's %q", filters, s)
		}
	}
	// Documents why a single "video.chunks.*.{id}" filter is not enough.
	if subjectMatches("video.chunks.*."+vid, SubjectHlsReady(vid)) {
		t.Error("single-token wildcard unexpectedly matched hls.ready")
	}
	if !subjectMatches(DLQFilterSubjects(vid)[0], SubjectDLQ(ConsumerAI, vid)) {
		t.Error("DLQ filter misses dlq.ai-indexer.{id}")
	}
}

func TestMsgIDs(t *testing.T) {
	cases := []struct{ got, want string }{
		{MsgIDUploaded(vid, 5), "uploaded:" + vid + ":000005"},
		{MsgIDHlsReady(vid, RenditionSrc, 5), "hls-ready:" + vid + ":src:000005"},
		{MsgIDAIIndexed(vid, 42), "ai-indexed:" + vid + ":000042"},
		{MsgIDCompleted(vid), "completed:" + vid},
		{MsgIDDLQ(ConsumerAI, 1234), "dlq:ai-indexer:1234"},
		{MsgIDRedrive(MsgIDUploaded(vid, 7), 1), "uploaded:" + vid + ":000007:redrive:1"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestObjectKeys(t *testing.T) {
	cases := []struct{ got, want string }{
		{RawChunk(vid, 42).String(), "fp-raw/videos/" + vid + "/chunks/000042.ts"},
		{HlsSegment(vid, "src", 42).String(), "fp-media/videos/" + vid + "/hls/src/000042.ts"},
		{Thumbnail(vid, 42).String(), "fp-media/videos/" + vid + "/thumbs/000042.jpg"},
		{Transcript(vid, 42).String(), "fp-media/videos/" + vid + "/transcripts/000042.json"},
		{MediaPlaylist(vid, "src").String(), "fp-media/videos/" + vid + "/hls/src/playlist.m3u8"},
		{MasterPlaylist(vid).String(), "fp-media/videos/" + vid + "/hls/master.m3u8"},
		{ThumbnailKeyPayload(vid, 42), "videos/" + vid + "/thumbs/000042.jpg"},
		{HlsIndexKey(vid, "src"), vid + ".src"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestParseObject(t *testing.T) {
	o, err := ParseObject("fp-raw/videos/" + vid + "/chunks/000000.ts")
	if err != nil || o != RawChunk(vid, 0) {
		t.Fatalf("ParseObject = %+v, %v", o, err)
	}
	for _, bad := range []string{
		"fp-raw",
		"fp-raw/",
		"other-bucket/videos/x",
		"fp-media/videos/../../etc/passwd",
		"fp-media//videos/x",
		"fp-media/videos//x",
		"fp-media/videos/./x",
		`fp-media/videos\..\x`,
	} {
		if _, err := ParseObject(bad); err == nil {
			t.Errorf("ParseObject(%q) accepted", bad)
		}
	}
}

func TestAPIPaths(t *testing.T) {
	cases := []struct{ got, want string }{
		{APIMasterPlaylist(vid), "/api/v1/videos/" + vid + "/hls/master.m3u8"},
		{APIMediaPlaylist(vid, "src"), "/api/v1/videos/" + vid + "/hls/src/playlist.m3u8"},
		{APISegment(vid, "src", 0), "/api/v1/videos/" + vid + "/hls/src/000000.ts"},
		{APIThumbnail(vid, 42), "/api/v1/videos/" + vid + "/thumbs/000042.jpg"},
		{APISearch("architecture", 10), "/api/v1/search?limit=10&q=architecture"},
		{APISearch("data pipeline & co", 0), "/api/v1/search?q=data+pipeline+%26+co"},
		{APISearchLegacy("architecture", 0), "/api/search?q=architecture"},
		// Path escaping keeps hostile IDs inside one path segment.
		{APIMasterPlaylist("../x"), "/api/v1/videos/..%2Fx/hls/master.m3u8"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func TestVideoIDFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		id, err := NewVideoID()
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateVideoID(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateVideoID(vid); err != nil {
		t.Errorf("doc example rejected: %v", err)
	}
	for _, bad := range []string{
		strings.ToUpper(vid),                   // uppercase
		"0192f3a4-7b1c-4d2e-9f00-1a2b3c4d5e6f", // v4
		"0192f3a47b1c7d2e9f001a2b3c4d5e6f",     // no hyphens
		"{" + vid + "}",
		"0192f3a4-7b1c-7d2e-cf00-1a2b3c4d5e6f", // wrong variant
		"",
		vid + "\n",
	} {
		if ValidateVideoID(bad) == nil {
			t.Errorf("ValidateVideoID(%q) accepted", bad)
		}
	}
}

func TestVideoIDsSortByCreation(t *testing.T) {
	a, _ := NewVideoID()
	b, _ := NewVideoID()
	if !(a < b) {
		t.Errorf("UUIDv7 not monotonic in string order: %s >= %s", a, b)
	}
}

func TestPointID(t *testing.T) {
	ns := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8") // any fixed namespace
	p1 := PointID(ns, vid, 42)
	p2 := PointID(ns, vid, 42)
	if p1 != p2 {
		t.Fatal("point ID not deterministic")
	}
	if p1.Version() != 5 || p1.Variant() != uuid.RFC4122 {
		t.Fatalf("point ID %s: version %d variant %v, want v5 RFC4122", p1, p1.Version(), p1.Variant())
	}
	if p1 == PointID(ns, vid, 43) {
		t.Fatal("different chunks share a point ID")
	}
	// Must hash the seq6 form, not the bare number.
	if p1 != uuid.NewSHA1(ns, []byte(vid+":000042")) {
		t.Fatal("point ID is not UUIDv5 over {video_id}:{seq6}")
	}
	byName, err := PointIDFromName(ns, vid+":000042")
	if err != nil || byName != p1 {
		t.Fatalf("PointIDFromName = %s, %v", byName, err)
	}
	for _, bad := range []string{vid + ":42", "V:000042", vid} {
		if _, err := PointIDFromName(ns, bad); err == nil {
			t.Errorf("PointIDFromName(%q) accepted", bad)
		}
	}
}

func TestFormatTimestampRange(t *testing.T) {
	cases := []struct {
		s, e int64
		want string
	}{
		{85000, 100000, "00:01:25 - 00:01:40"}, // Feature 6 example
		{85999, 100001, "00:01:25 - 00:01:41"}, // floor start, ceil end
		{0, 1, "00:00:00 - 00:00:01"},
		{3_599_000, 3_661_500, "00:59:59 - 01:01:02"},
		{86_399_000, 86_400_000, "23:59:59 - 24:00:00"}, // 24 h session limit
	}
	for _, c := range cases {
		if got := FormatTimestampRange(c.s, c.e); got != c.want {
			t.Errorf("FormatTimestampRange(%d,%d) = %q, want %q", c.s, c.e, got, c.want)
		}
	}
}
