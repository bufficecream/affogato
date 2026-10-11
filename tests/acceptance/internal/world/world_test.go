package world

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

func TestVideoAliasesAreStableUUIDv7(t *testing.T) {
	w := New(&Suite{}, "s")
	v1, err := w.Video("V")
	if err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidateVideoID(v1); err != nil {
		t.Fatal(err)
	}
	v2, _ := w.Video("V")
	wid, _ := w.Video("W")
	if v1 != v2 || v1 == wid {
		t.Fatalf("aliases: V=%s V'=%s W=%s", v1, v2, wid)
	}
	if New(&Suite{}, "other").mustVideo(t, "V") == v1 {
		t.Fatal("two scenarios share a video_id")
	}
	for _, bad := range []string{"", "v", "VV", "1"} {
		if _, err := w.Video(bad); err == nil {
			t.Errorf("alias %q accepted", bad)
		}
	}
}

func (w *World) mustVideo(t *testing.T, a string) string {
	id, err := w.Video(a)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestExpand(t *testing.T) {
	w := New(&Suite{}, "s")
	v := w.mustVideo(t, "V")
	wid := w.mustVideo(t, "W")
	cases := map[string]string{
		"af-raw/videos/V/chunks/000000.ts":       "af-raw/videos/" + v + "/chunks/000000.ts",
		"video.chunks.uploaded.V":                "video.chunks.uploaded." + v,
		"dlq.hls-stream-worker.V":                "dlq.hls-stream-worker." + v,
		"V.src":                                  v + ".src",
		`UUIDv5("V:000042")`:                     `UUIDv5("` + v + `:000042")`,
		"/api/v1/videos/V/hls/src/playlist.m3u8": "/api/v1/videos/" + v + "/hls/src/playlist.m3u8",
		"videos/V/thumbs/000042.jpg":             "videos/" + v + "/thumbs/" + "000042.jpg",
		"/api/v1/videos/W/hls/master.m3u8":       "/api/v1/videos/" + wid + "/hls/master.m3u8",
		"V":                                      v,
		// Must stay untouched.
		"#EXT-X-PLAYLIST-TYPE:VOD":      "#EXT-X-PLAYLIST-TYPE:VOD",
		"#EXT-X-ENDLIST":                "#EXT-X-ENDLIST",
		"ERROR_CLASS_INVALID_MEDIA":     "ERROR_CLASS_INVALID_MEDIA",
		"video/mp2t":                    "video/mp2t",
		"/api/v1/search?q=architecture": "/api/v1/search?q=architecture",
		"src/playlist.m3u8":             "src/playlist.m3u8",
		"application/vnd.apple.mpegurl": "application/vnd.apple.mpegurl",
	}
	for in, want := range cases {
		got, err := w.Expand(in)
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestExpandUnknownVideo(t *testing.T) {
	w := New(&Suite{}, "s")
	got, err := w.Expand("/api/v1/videos/{an unknown UUIDv7}/hls/master.m3u8")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(got, "/api/v1/videos/"), "/hls/master.m3u8")
	if err := contracts.ValidateVideoID(id); err != nil {
		t.Fatalf("placeholder expanded to %q: %v", id, err)
	}
	if len(w.Aliases()) != 0 {
		t.Fatal("unknown video registered as an alias")
	}
}

func TestCleanupsRunLIFOAndJoinErrors(t *testing.T) {
	w := New(&Suite{}, "s")
	var order []string
	w.AddCleanup("first", func(context.Context) error { order = append(order, "first"); return nil })
	w.AddCleanup("second", func(context.Context) error { order = append(order, "second"); return errors.New("x") })
	w.AddCleanup("third", func(context.Context) error { order = append(order, "third"); return nil })
	err := w.Close(context.Background())
	if err == nil || !strings.Contains(err.Error(), "cleanup second") {
		t.Fatalf("Close error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"third", "second", "first"}) {
		t.Fatalf("order = %v", order)
	}
	order = nil
	if err := w.Close(context.Background()); err != nil || order != nil {
		t.Fatal("cleanups ran twice")
	}
}

func TestContext(t *testing.T) {
	w := New(&Suite{}, "s")
	if From(With(context.Background(), w)) != w {
		t.Fatal("round trip failed")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("From without World did not panic")
		}
	}()
	From(context.Background())
}

func TestFaultsWithoutInjector(t *testing.T) {
	if _, err := New(&Suite{}, "s").Faults(); err == nil {
		t.Fatal("expected error without injector")
	}
}
