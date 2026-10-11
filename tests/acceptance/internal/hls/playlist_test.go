package hls

import (
	"reflect"
	"strings"
	"testing"
)

// Shape of the playlist expected for Feature 1 "corrupted chunk": chunk 3 FAILED.
const vodWithGap = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-TARGETDURATION:3
#EXT-X-MEDIA-SEQUENCE:0
#EXTINF:2.002,
000000.ts
#EXTINF:2.002,
000001.ts
#EXTINF:1.968,
000002.ts
#EXT-X-DISCONTINUITY
#EXTINF:2.035,
000004.ts
#EXTINF:1.000,
000005.ts
#EXT-X-ENDLIST
`

func TestParseMediaVOD(t *testing.T) {
	p, err := ParseMedia([]byte(vodWithGap))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.CheckVOD(); err != nil {
		t.Fatal(err)
	}
	want := []string{"000000.ts", "000001.ts", "000002.ts", "000004.ts", "000005.ts"}
	if !reflect.DeepEqual(p.URIs(), want) {
		t.Fatalf("URIs = %v", p.URIs())
	}
	s, ok := p.Find("000004.ts")
	if !ok || !s.DiscontinuityBefore || s.Duration != 2.035 {
		t.Fatalf("000004.ts = %+v", s)
	}
	if s, _ := p.Find("000005.ts"); s.DiscontinuityBefore {
		t.Fatal("discontinuity leaked to the next segment")
	}
	if _, ok := p.Find("000003.ts"); ok {
		t.Fatal("failed chunk present")
	}
}

func TestCheckVODRejects(t *testing.T) {
	cases := map[string]string{
		"no endlist":        strings.Replace(vodWithGap, "#EXT-X-ENDLIST\n", "", 1),
		"event type":        strings.Replace(vodWithGap, "TYPE:VOD", "TYPE:EVENT", 1),
		"wrong target":      strings.Replace(vodWithGap, "TARGETDURATION:3", "TARGETDURATION:2", 1),
		"absolute uri":      strings.Replace(vodWithGap, "000000.ts", "http://minio:9000/fp-media/x/000000.ts", 1),
		"host-relative uri": strings.Replace(vodWithGap, "000000.ts", "/api/v1/videos/x/hls/src/000000.ts", 1),
		"traversal uri":     strings.Replace(vodWithGap, "000000.ts", "../other/000000.ts", 1),
		"leading disc":      strings.Replace(vodWithGap, "#EXT-X-MEDIA-SEQUENCE:0\n", "#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-DISCONTINUITY\n", 1),
	}
	for name, body := range cases {
		p, err := ParseMedia([]byte(body))
		if err != nil {
			continue // a parse error is also a rejection
		}
		if err := p.CheckVOD(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseMediaErrors(t *testing.T) {
	for name, body := range map[string]string{
		"no header":       "#EXTINF:2,\n000000.ts\n",
		"uri without inf": "#EXTM3U\n000000.ts\n",
		"bad duration":    "#EXTM3U\n#EXTINF:abc,\n000000.ts\n",
		"zero duration":   "#EXTM3U\n#EXTINF:0,\n000000.ts\n",
		"trailing inf":    "#EXTM3U\n#EXTINF:2,\n",
		"after endlist":   "#EXTM3U\n#EXT-X-ENDLIST\n#EXTINF:2,\n000000.ts\n",
		"empty":           "",
	} {
		if _, err := ParseMedia([]byte(body)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestParseMasterAndResolve(t *testing.T) {
	m, err := ParseMaster([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=6000000,RESOLUTION=1920x1080,CODECS=\"avc1.640028,mp4a.40.2\"\nsrc/playlist.m3u8\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Variants) != 1 || m.Variants[0].URI != "src/playlist.m3u8" || m.Variants[0].Bandwidth != 6000000 {
		t.Fatalf("variants = %+v", m.Variants)
	}
	got, err := ResolveVariant("http://api:8080/api/v1/videos/V/hls/master.m3u8", m.Variants[0].URI)
	if err != nil || got != "http://api:8080/api/v1/videos/V/hls/src/playlist.m3u8" {
		t.Fatalf("resolved = %q, %v", got, err)
	}
	seg, _ := ResolveVariant(got, "000000.ts")
	if seg != "http://api:8080/api/v1/videos/V/hls/src/000000.ts" {
		t.Fatalf("segment resolved to %q", seg)
	}
}

func TestExpectedDiscontinuities(t *testing.T) {
	got := ExpectedDiscontinuities([]uint32{0, 1, 3, 4, 7})
	want := map[uint32]bool{3: true, 7: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(ExpectedDiscontinuities([]uint32{0, 1, 2})) != 0 {
		t.Fatal("contiguous chunks need no discontinuity")
	}
}
