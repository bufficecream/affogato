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
