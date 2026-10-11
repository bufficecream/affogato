package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDoesNotFollowRedirects(t *testing.T) {
	var targetHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/videos/x/thumbs/000000.jpg":
			http.Redirect(w, r, "http://minio.example:9000/fp-media/videos/x/thumbs/000000.jpg?X-Amz-Expires=600", http.StatusFound)
		case "/relative":
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusFound)
		case "/target":
			targetHits++
			w.WriteHeader(http.StatusOK)
		case "/conflict":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"status":"processing"}`))
		case "/gone":
			w.WriteHeader(http.StatusGone)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL, 5*time.Second, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	r, err := c.Get(ctx, "/api/v1/videos/x/thumbs/000000.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ExpectStatus(http.StatusFound); err != nil {
		t.Fatal(err)
	}
	loc, err := r.Location()
	if err != nil || loc.Host != "minio.example:9000" {
		t.Fatalf("Location = %v, %v", loc, err)
	}

	r, err = c.Get(ctx, "/relative")
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusFound || targetHits != 0 {
		t.Fatalf("redirect followed: status %d, target hits %d", r.StatusCode, targetHits)
	}
	if loc, _ := r.Location(); loc.String() != srv.URL+"/target" {
		t.Fatalf("relative Location not resolved: %v", loc)
	}

	r, err = c.Get(ctx, "/conflict")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []error{
		r.ExpectStatus(http.StatusConflict),
		r.ExpectMediaType("application/json"),
		r.ExpectHeader("Cache-Control", "no-store"),
		r.ExpectHeader("Retry-After", "5"),
	} {
		if check != nil {
			t.Error(check)
		}
	}
	if err := r.ExpectStatus(200); err == nil || !strings.Contains(err.Error(), "processing") {
		t.Errorf("status mismatch error should include body, got %v", err)
	}

	r, err = c.Get(ctx, "/gone")
	if err != nil || r.StatusCode != http.StatusGone {
		t.Fatalf("410 not surfaced: %v %v", r, err)
	}
	if _, err := r.Location(); err == nil {
		t.Error("Location on a response without one should fail")
	}
}

func TestBodyCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, time.Second, 1024)
	if _, err := c.Get(context.Background(), "/"); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("want ErrBodyTooLarge, got %v", err)
	}
	c, _ = New(srv.URL, time.Second, 2048)
	if r, err := c.Get(context.Background(), "/"); err != nil || len(r.Body) != 2048 {
		t.Fatalf("exact-size body rejected: %v", err)
	}
}

func TestNewValidates(t *testing.T) {
	if _, err := New("localhost:8080", time.Second, 1); err == nil {
		t.Error("relative base URL accepted")
	}
	if _, err := New("http://x", time.Second, 0); err == nil {
		t.Error("zero body cap accepted")
	}
}

func presigned(host, path string, date time.Time, expires string) *url.URL {
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", "minio/20261011/us-east-1/s3/aws4_request")
	q.Set("X-Amz-Date", date.UTC().Format("20060102T150405Z"))
	q.Set("X-Amz-Expires", expires)
	q.Set("X-Amz-SignedHeaders", "host")
	q.Set("X-Amz-Signature", "deadbeef")
	return &url.URL{Scheme: "http", Host: host, Path: path, RawQuery: q.Encode()}
}

func TestCheckPresignedURL(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	chk := PresignCheck{
		PublicEndpoint: "http://minio.localtest.me:9000",
		Bucket:         "fp-media", Key: "videos/v/thumbs/000042.jpg",
		MaxTTL: 10 * time.Minute, Now: now, ClockSkew: 30 * time.Second,
	}
	good := presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now, "600")
	if err := CheckPresignedURL(good, chk); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	bad := map[string]*url.URL{
		"internal host":    presigned("framepulse-minio.framepulse.svc:9000", "/fp-media/videos/v/thumbs/000042.jpg", now, "600"),
		"ttl too long":     presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now, "3600"),
		"wrong object":     presigned("minio.localtest.me:9000", "/fp-media/videos/other/thumbs/000042.jpg", now, "600"),
		"already expired":  presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now.Add(-time.Hour), "600"),
		"signed in future": presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now.Add(time.Hour), "600"),
	}
	for name, u := range bad {
		if err := CheckPresignedURL(u, chk); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	missing := presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now, "600")
	q := missing.Query()
	q.Del("X-Amz-Signature")
	missing.RawQuery = q.Encode()
	if err := CheckPresignedURL(missing, chk); err == nil {
		t.Error("unsigned URL accepted")
	}
	listing := presigned("minio.localtest.me:9000", "/fp-media/videos/v/thumbs/000042.jpg", now, "600")
	q = listing.Query()
	q.Set("list-type", "2")
	listing.RawQuery = q.Encode()
	if err := CheckPresignedURL(listing, chk); err == nil {
		t.Error("listing parameter accepted")
	}
}
