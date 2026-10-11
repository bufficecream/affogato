package qdrant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeQdrant(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/collections/transcripts/points/11111111-1111-5111-8111-111111111111":
			_, _ = io.WriteString(w, `{"result":{"id":"11111111-1111-5111-8111-111111111111","payload":{"video_id":"v","start_ms":85000,"thumbnail_key":"videos/v/thumbs/000042.jpg"}},"status":"ok"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/collections/transcripts/points/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"status":{"error":"Not found: Point with id x does not exists!"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/collections/transcripts/points/count":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["exact"] != true {
				t.Errorf("count not exact: %v", body)
			}
			_, _ = io.WriteString(w, `{"result":{"count":8}}`)
		case r.URL.Path == "/aliases":
			_, _ = io.WriteString(w, `{"result":{"aliases":[{"alias_name":"transcripts","collection_name":"transcripts_me5s384_v1"}]}}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

func TestClient(t *testing.T) {
	srv := fakeQdrant(t)
	defer srv.Close()
	c, err := New(srv.URL, "k", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	p, found, err := c.GetPoint(ctx, "transcripts", "11111111-1111-5111-8111-111111111111")
	if err != nil || !found {
		t.Fatalf("GetPoint: %v %v", found, err)
	}
	if n, ok := PayloadInt(p, "start_ms"); !ok || n != 85000 {
		t.Errorf("start_ms = %d %v", n, ok)
	}
	if s, ok := PayloadString(p, "thumbnail_key"); !ok || s != "videos/v/thumbs/000042.jpg" {
		t.Errorf("thumbnail_key = %q", s)
	}
	if _, ok := PayloadInt(p, "thumbnail_key"); ok {
		t.Error("string read as int")
	}

	if _, found, err := c.GetPoint(ctx, "transcripts", "22222222-2222-5222-8222-222222222222"); err != nil || found {
		t.Fatalf("missing point: found=%v err=%v", found, err)
	}
	if n, err := c.CountByVideo(ctx, "transcripts", "v"); err != nil || n != 8 {
		t.Fatalf("count = %d %v", n, err)
	}
	if col, err := c.AliasTarget(ctx, "transcripts"); err != nil || col != "transcripts_me5s384_v1" {
		t.Fatalf("alias = %q %v", col, err)
	}
	if col, _ := c.AliasTarget(ctx, "nope"); col != "" {
		t.Fatalf("unknown alias resolved to %q", col)
	}

	bad, _ := New(srv.URL, "wrong", time.Second)
	if _, _, err := bad.GetPoint(ctx, "transcripts", "x"); err == nil {
		t.Fatal("401 not surfaced as error")
	}
}
