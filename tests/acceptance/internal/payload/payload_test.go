package payload

import (
	"errors"
	"testing"
)

func TestPendingCodecReportsNotImplemented(t *testing.T) {
	var c Codec = PendingCodec{}
	if _, err := c.DecodeHlsIndex(nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
	if _, err := c.EncodeVideoChunkUploaded(VideoChunkUploaded{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestHlsIndexHelpers(t *testing.T) {
	x := HlsIndex{Segments: []Segment{
		{ChunkSeq: 0, Status: SegmentReady},
		{ChunkSeq: 1, Status: SegmentReady},
		{ChunkSeq: 3, Status: SegmentFailed},
	}}
	if !x.SegmentsInOrder() {
		t.Error("sorted index reported unsorted")
	}
	if r, f := x.Counts(); r != 2 || f != 1 {
		t.Errorf("Counts = %d,%d", r, f)
	}
	if _, ok := x.Segment(2); ok {
		t.Error("found missing seq 2")
	}
	dup := HlsIndex{Segments: []Segment{{ChunkSeq: 5}, {ChunkSeq: 5}}}
	if dup.SegmentsInOrder() || dup.CountSeq(5) != 2 {
		t.Error("duplicate entries not detected")
	}
	unsorted := HlsIndex{Segments: []Segment{{ChunkSeq: 2}, {ChunkSeq: 0}}}
	if unsorted.SegmentsInOrder() {
		t.Error("unsorted index reported sorted")
	}
}

func TestHlsIndexCap(t *testing.T) {
	full := HlsIndex{VideoID: "v", Rendition: "src", ExpectedChunkCount: 8000}
	for i := uint32(0); i < 8000; i++ {
		full.Segments = append(full.Segments, Segment{ChunkSeq: i, Status: SegmentReady})
	}
	if err := full.CheckCap(); err != nil {
		t.Fatalf("8,000-segment index rejected: %v", err)
	}
	over := full
	over.Segments = append(append([]Segment(nil), full.Segments...), Segment{ChunkSeq: 8000})
	if over.CheckCap() == nil {
		t.Error("8,001 segments / chunk_seq 8000 accepted")
	}
	badSeq := HlsIndex{Segments: []Segment{{ChunkSeq: 8000}}}
	if badSeq.CheckCap() == nil {
		t.Error("chunk_seq 8000 accepted")
	}
	badCount := HlsIndex{ExpectedChunkCount: 8001}
	if badCount.CheckCap() == nil {
		t.Error("expected_chunk_count 8001 accepted")
	}
	if err := CheckEncodedSize(make([]byte, MaxHlsIndexBytes)); err != nil {
		t.Errorf("value at the budget rejected: %v", err)
	}
	if CheckEncodedSize(make([]byte, MaxHlsIndexBytes+1)) == nil {
		t.Error("value above the budget accepted")
	}
}
