package contracts

import (
	"reflect"
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
		{RawChunk(vid, 42).String(), "af-raw/videos/" + vid + "/chunks/000042.ts"},
		{HlsSegment(vid, "src", 42).String(), "af-media/videos/" + vid + "/hls/src/000042.ts"},
		{Thumbnail(vid, 42).String(), "af-media/videos/" + vid + "/thumbs/000042.jpg"},
		{Transcript(vid, 42).String(), "af-media/videos/" + vid + "/transcripts/000042.json"},
		{MediaPlaylist(vid, "src").String(), "af-media/videos/" + vid + "/hls/src/playlist.m3u8"},
		{MasterPlaylist(vid).String(), "af-media/videos/" + vid + "/hls/master.m3u8"},
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
	o, err := ParseObject("af-raw/videos/" + vid + "/chunks/000000.ts")
	if err != nil || o != RawChunk(vid, 0) {
		t.Fatalf("ParseObject = %+v, %v", o, err)
	}
	for _, bad := range []string{
		"af-raw",
		"af-raw/",
		"other-bucket/videos/x",
		"af-media/videos/../../etc/passwd",
		"af-media//videos/x",
		"af-media/videos//x",
		"af-media/videos/./x",
		`af-media/videos\..\x`,
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
	if PointIDNamespace.String() != "c1907ffe-0c06-4077-8866-08fb8d658b77" {
		t.Fatalf("point-ID namespace %s, want the fixed value from spec A18 / review B3", PointIDNamespace)
	}
	p1 := PointID(vid, 42)
	if p1 != PointID(vid, 42) {
		t.Fatal("point ID not deterministic")
	}
	if p1.Version() != 5 || p1.Variant() != uuid.RFC4122 {
		t.Fatalf("point ID %s: version %d variant %v, want v5 RFC4122", p1, p1.Version(), p1.Variant())
	}
	if p1 == PointID(vid, 43) {
		t.Fatal("different chunks share a point ID")
	}
	// Must hash the seq6 form, not the bare number.
	if p1 != uuid.NewSHA1(PointIDNamespace, []byte(vid+":000042")) {
		t.Fatal("point ID is not UUIDv5 over {video_id}:{seq6}")
	}
	byName, err := PointIDFromName(vid + ":000042")
	if err != nil || byName != p1 {
		t.Fatalf("PointIDFromName = %s, %v", byName, err)
	}
	for _, bad := range []string{vid + ":42", "V:000042", vid, strings.ToUpper(vid) + ":000042"} {
		if _, err := PointIDFromName(bad); err == nil {
			t.Errorf("PointIDFromName(%q) accepted", bad)
		}
	}
}

// Golden values from review B3 (shared with the Go/Rust/Python contracts libraries via
// contracts/testdata/keys.json once it exists).
func TestPointIDGolden(t *testing.T) {
	const goldenVid = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"
	cases := []struct {
		seq  uint32
		want string
	}{
		{0, "ce06a6cc-f9c3-5d9f-8bd0-c7212687ea4d"},
		{42, "b5e27970-263d-5af6-ba86-f5345023df69"},
	}
	for _, c := range cases {
		if got := PointID(goldenVid, c.seq).String(); got != c.want {
			t.Errorf("PointID(%s, %d) = %s, want %s", goldenVid, c.seq, got, c.want)
		}
		got, err := PointIDFromName(goldenVid + ":" + Seq6(c.seq))
		if err != nil || got.String() != c.want {
			t.Errorf("PointIDFromName(%s:%s) = %s, %v, want %s", goldenVid, Seq6(c.seq), got, err, c.want)
		}
	}
}

// Review B1 "Per-video filters": exactly these four subjects, three on VIDEO_EVENTS and one
// on VIDEO_DLQ. Drift here silently drops events from the waiters.
func TestFilterSubjectsAreTheFourDocumented(t *testing.T) {
	const v = "0192f3a4-7b1c-7d2e-9f00-1a2b3c4d5e6f"
	wantEvents := []string{"video.chunks.*." + v, "video.chunks.*.*." + v, "video.lifecycle.*." + v}
	wantDLQ := []string{"dlq.*." + v}
	if got := EventFilterSubjects(v); !reflect.DeepEqual(got, wantEvents) {
		t.Errorf("EventFilterSubjects = %v, want %v", got, wantEvents)
	}
	if got := DLQFilterSubjects(v); !reflect.DeepEqual(got, wantDLQ) {
		t.Errorf("DLQFilterSubjects = %v, want %v", got, wantDLQ)
	}
	// dlq.{consumer}.unknown (undecodable payload) is documented as not matchable per video.
	if subjectMatches(DLQFilterSubjects(v)[0], SubjectDLQ(ConsumerAI, "unknown")) {
		t.Error("per-video DLQ filter matches dlq.ai-indexer.unknown")
	}
}

func TestVideoCap(t *testing.T) {
	if MaxVideoChunks != 8000 || MaxVideoDuration.Hours() != 4 {
		t.Fatalf("cap = %d chunks / %s, want 8000 / 4h (spec A13)", MaxVideoChunks, MaxVideoDuration)
	}
	for _, ok := range []int64{0, 1, 42, 7999} {
		if err := ValidateChunkSeq(ok); err != nil {
			t.Errorf("ValidateChunkSeq(%d): %v", ok, err)
		}
	}
	for _, bad := range []int64{-1, 8000, 8001, MaxChunkSeq, MaxChunkSeq + 1} {
		if err := ValidateChunkSeq(bad); err == nil {
			t.Errorf("ValidateChunkSeq(%d) accepted", bad)
		}
	}
	for _, ok := range []int64{1, 6, 8000} {
		if err := ValidateChunkCount(ok); err != nil {
			t.Errorf("ValidateChunkCount(%d): %v", ok, err)
		}
	}
	for _, bad := range []int64{0, -1, 8001} {
		if err := ValidateChunkCount(bad); err == nil {
			t.Errorf("ValidateChunkCount(%d) accepted", bad)
		}
	}
	// Key builders still format up to the seq6 limit (B2: builders reject > 999,999 only).
	if got := RawChunk(vid, 8000).Key; !strings.HasSuffix(got, "/chunks/008000.ts") {
		t.Errorf("RawChunk(8000) = %s", got)
	}
}

func TestTestFaultValue(t *testing.T) {
	cases := []struct {
		f        TestFault
		consumer string
		n        int
		want     string
	}{
		{FaultExitOnReceive, ConsumerAI, 0, "exit-on-receive; consumer=ai-indexer"},
		{FaultCrashBeforeAck, ConsumerHls, 0, "crash-before-ack; consumer=hls-stream-worker"},
		{FaultCrashBeforeAck, ConsumerAI, 0, "crash-before-ack; consumer=ai-indexer"},
		{FaultFailTransient, ConsumerAI, 3, "fail-transient; consumer=ai-indexer; deliveries=3"},
	}
	for _, c := range cases {
		got, err := TestFaultValue(c.f, c.consumer, c.n)
		if err != nil || got != c.want {
			t.Errorf("TestFaultValue(%s,%s,%d) = %q, %v; want %q", c.f, c.consumer, c.n, got, err, c.want)
			continue
		}
		p, err := ParseTestFaultValue(got)
		if err != nil || p != (ParsedTestFault{c.f, c.consumer, c.n}) {
			t.Errorf("ParseTestFaultValue(%q) = %+v, %v", got, p, err)
		}
	}
	if _, err := TestFaultValue("kill-9", ConsumerAI, 0); err == nil {
		t.Error("unknown fault accepted")
	}
	if _, err := TestFaultValue(FaultExitOnReceive, "embedder", 0); err == nil {
		t.Error("unknown consumer accepted")
	}
	if _, err := TestFaultValue(FaultExitOnReceive, ConsumerAI, -1); err == nil {
		t.Error("negative deliveries accepted")
	}
	// Malformed values a worker must dead-letter as TEST_FAULT_INVALID.
	for _, bad := range []string{
		"", "exit-on-receive", "exit-on-receive; consumer=", "exit-on-receive; consumer=other",
		"exit-on-receive; consumer=ai-indexer; deliveries=0", "exit-on-receive; consumer=ai-indexer; deliveries=-1",
		"exit-on-receive; consumer=ai-indexer; deliveries=01", "exit-on-receive; consumer=ai-indexer; deliveries=x",
		"exit-on-receive; consumer=ai-indexer; mode=hard", "exit-on-receive; consumer=ai-indexer; consumer=ai-indexer",
		"Exit-On-Receive; consumer=ai-indexer", "exit-on-receive; ai-indexer",
	} {
		if p, err := ParseTestFaultValue(bad); err == nil {
			t.Errorf("ParseTestFaultValue(%q) accepted: %+v", bad, p)
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
		{14_398_000, 14_400_000, "03:59:58 - 04:00:00"}, // end of a video at the 4 h cap (A13)
		{86_399_000, 86_400_000, "23:59:59 - 24:00:00"}, // format only; above the cap
	}
	for _, c := range cases {
		if got := FormatTimestampRange(c.s, c.e); got != c.want {
			t.Errorf("FormatTimestampRange(%d,%d) = %q, want %q", c.s, c.e, got, c.want)
		}
	}
}
