// Package hls parses the static playlists the Stream Worker writes so steps can assert on
// structure (order, EXTINF, discontinuities, ENDLIST) instead of substrings.
package hls

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
)

type Segment struct {
	URI                 string
	Duration            float64 // EXTINF seconds
	DiscontinuityBefore bool
}

type MediaPlaylist struct {
	Version        int
	TargetDuration int
	PlaylistType   string
	MediaSequence  int
	EndList        bool
	Segments       []Segment
	Tags           []string // every #EXT tag line, in order
}

type Variant struct {
	URI       string
	Bandwidth int
	Attrs     string
}

type MasterPlaylist struct {
	Variants []Variant
}

// ParseMedia parses a media playlist strictly enough for the VOD contract.
func ParseMedia(b []byte) (*MediaPlaylist, error) {
	lines, err := lines(b)
	if err != nil {
		return nil, err
	}
	p := &MediaPlaylist{}
	var pendingDur *float64
	pendingDisc := false
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "#EXT"):
			p.Tags = append(p.Tags, l)
			name, val, _ := strings.Cut(l, ":")
			switch name {
			case "#EXT-X-VERSION":
				p.Version, err = strconv.Atoi(val)
			case "#EXT-X-TARGETDURATION":
				p.TargetDuration, err = strconv.Atoi(val)
			case "#EXT-X-PLAYLIST-TYPE":
				p.PlaylistType = val
			case "#EXT-X-MEDIA-SEQUENCE":
				p.MediaSequence, err = strconv.Atoi(val)
			case "#EXT-X-ENDLIST":
				p.EndList = true
			case "#EXT-X-DISCONTINUITY":
				pendingDisc = true
			case "#EXTINF":
				durStr, _, _ := strings.Cut(val, ",")
				var d float64
				d, err = strconv.ParseFloat(durStr, 64)
				if err == nil && (d <= 0 || math.IsInf(d, 0) || math.IsNaN(d)) {
					err = fmt.Errorf("non-positive duration")
				}
				pendingDur = &d
			}
			if err != nil {
				return nil, fmt.Errorf("line %d %q: %w", i+1, l, err)
			}
		case strings.HasPrefix(l, "#"):
			// comment
		default:
			if pendingDur == nil {
				return nil, fmt.Errorf("line %d: URI %q without #EXTINF", i+1, l)
			}
			if p.EndList {
				return nil, fmt.Errorf("line %d: segment after #EXT-X-ENDLIST", i+1)
			}
			p.Segments = append(p.Segments, Segment{URI: l, Duration: *pendingDur, DiscontinuityBefore: pendingDisc})
			pendingDur, pendingDisc = nil, false
		}
	}
	if pendingDur != nil {
		return nil, fmt.Errorf("trailing #EXTINF without URI")
	}
	return p, nil
}

// ParseMaster parses a master playlist's variant streams.
func ParseMaster(b []byte) (*MasterPlaylist, error) {
	lines, err := lines(b)
	if err != nil {
		return nil, err
	}
	m := &MasterPlaylist{}
	var pending *Variant
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "#EXT-X-STREAM-INF:"):
			attrs := strings.TrimPrefix(l, "#EXT-X-STREAM-INF:")
			v := &Variant{Attrs: attrs}
			for _, kv := range splitAttrs(attrs) {
				k, val, _ := strings.Cut(kv, "=")
				if k == "BANDWIDTH" {
					v.Bandwidth, _ = strconv.Atoi(val)
				}
			}
			pending = v
		case strings.HasPrefix(l, "#"):
		default:
			if pending == nil {
				return nil, fmt.Errorf("line %d: URI %q without #EXT-X-STREAM-INF", i+1, l)
			}
			pending.URI = l
			m.Variants = append(m.Variants, *pending)
			pending = nil
		}
	}
	return m, nil
}

func lines(b []byte) ([]string, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	var out []string
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" {
			out = append(out, l)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 || out[0] != "#EXTM3U" {
		return nil, fmt.Errorf("playlist does not start with #EXTM3U")
	}
	return out, nil
}

// splitAttrs splits an attribute list on commas outside quotes.
func splitAttrs(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
			cur.WriteRune(r)
		case r == ',' && !inQ:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// URIs returns segment URIs in playlist order.
func (p *MediaPlaylist) URIs() []string {
	out := make([]string, len(p.Segments))
	for i, s := range p.Segments {
		out[i] = s.URI
	}
	return out
}

// Find returns the segment with the given URI.
func (p *MediaPlaylist) Find(uri string) (Segment, bool) {
	for _, s := range p.Segments {
		if s.URI == uri {
			return s, true
		}
	}
	return Segment{}, false
}

// CheckVOD verifies the finalization contract (spec 2.2): VOD type, ENDLIST, relative
// URIs only, TARGETDURATION = ceil(max EXTINF), and no discontinuity before the first segment.
func (p *MediaPlaylist) CheckVOD() error {
	if p.PlaylistType != "VOD" {
		return fmt.Errorf("#EXT-X-PLAYLIST-TYPE = %q, want VOD", p.PlaylistType)
	}
	if !p.EndList {
		return fmt.Errorf("missing #EXT-X-ENDLIST")
	}
	if len(p.Segments) == 0 {
		return fmt.Errorf("no segments")
	}
	maxDur := 0.0
	for i, s := range p.Segments {
		if err := CheckRelativeURI(s.URI); err != nil {
			return fmt.Errorf("segment %d: %w", i, err)
		}
		if s.Duration > maxDur {
			maxDur = s.Duration
		}
	}
	if p.Segments[0].DiscontinuityBefore {
		return fmt.Errorf("#EXT-X-DISCONTINUITY before the first segment")
	}
	if want := int(math.Ceil(maxDur)); p.TargetDuration != want {
		return fmt.Errorf("#EXT-X-TARGETDURATION = %d, want ceil(max EXTINF %.3f) = %d", p.TargetDuration, maxDur, want)
	}
	return nil
}

// CheckRelativeURI rejects absolute URLs, host-relative paths and parent traversal, so a
// proxied playlist can only reach the API's own segment routes.
func CheckRelativeURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return err
	}
	if u.IsAbs() || u.Host != "" || strings.HasPrefix(uri, "/") {
		return fmt.Errorf("URI %q is not relative", uri)
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == ".." {
			return fmt.Errorf("URI %q traverses upwards", uri)
		}
	}
	return nil
}

// ExpectedDiscontinuities returns, for READY chunk seqs in order, which ones must carry
// #EXT-X-DISCONTINUITY: every READY chunk that follows a gap (FAILED or missing chunk).
func ExpectedDiscontinuities(readySeqs []uint32) map[uint32]bool {
	out := map[uint32]bool{}
	for i := 1; i < len(readySeqs); i++ {
		if readySeqs[i] != readySeqs[i-1]+1 {
			out[readySeqs[i]] = true
		}
	}
	return out
}

// ResolveVariant resolves a master playlist's variant URI against the URL the master was
// fetched from, the way a player does.
func ResolveVariant(masterURL, variantURI string) (string, error) {
	base, err := url.Parse(masterURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(variantURI)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(ref).String(), nil
}
