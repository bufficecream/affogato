package s3assert

import (
	"strings"
	"testing"

	"github.com/minio/minio-go/v7/pkg/lifecycle"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

func objectFor(key string) contracts.Object {
	return contracts.Object{Bucket: contracts.BucketMedia, Key: key}
}

func TestCompareBytes(t *testing.T) {
	if err := CompareBytes([]byte("abc"), []byte("abc")); err != nil {
		t.Fatal(err)
	}
	err := CompareBytes([]byte("#EXTM3U\nA"), []byte("#EXTM3U\nB"))
	if err == nil || !strings.Contains(err.Error(), "offset 8") {
		t.Fatalf("diff offset: %v", err)
	}
	err = CompareBytes([]byte("abc"), []byte("abcd"))
	if err == nil || !strings.Contains(err.Error(), "offset 3") || !strings.Contains(err.Error(), "<eof>") {
		t.Fatalf("length diff: %v", err)
	}
	if CompareBytes(nil, []byte{}) != nil {
		t.Fatal("nil vs empty should be equal")
	}
}

func TestSameMediaType(t *testing.T) {
	if err := SameMediaType("application/json; charset=utf-8", "application/json"); err != nil {
		t.Error(err)
	}
	if err := SameMediaType("Video/MP2T", "video/mp2t"); err != nil {
		t.Error(err)
	}
	if err := SameMediaType("application/octet-stream", "video/mp2t"); err == nil {
		t.Error("mismatch accepted")
	}
	if err := SameMediaType("", "video/mp2t"); err == nil {
		t.Error("empty accepted")
	}
}

func TestExpirationDaysFrom(t *testing.T) {
	raw := &lifecycle.Configuration{Rules: []lifecycle.Rule{
		{ID: "expire-raw", Status: "Enabled", Expiration: lifecycle.Expiration{Days: 10}},
		{ID: "abort-mpu", Status: "Enabled", AbortIncompleteMultipartUpload: lifecycle.AbortIncompleteMultipartUpload{DaysAfterInitiation: 1}},
	}}
	if d, err := ExpirationDaysFrom(raw); err != nil || d != 10 {
		t.Fatalf("af-raw days = %d, %v", d, err)
	}
	media := &lifecycle.Configuration{Rules: []lifecycle.Rule{
		{ID: "abort-mpu", Status: "Enabled", AbortIncompleteMultipartUpload: lifecycle.AbortIncompleteMultipartUpload{DaysAfterInitiation: 1}},
	}}
	if d, err := ExpirationDaysFrom(media); err != nil || d != 0 {
		t.Fatalf("af-media days = %d, %v", d, err)
	}
	if d, _ := ExpirationDaysFrom(nil); d != 0 {
		t.Fatal("nil config should mean no expiry")
	}
	disabled := &lifecycle.Configuration{Rules: []lifecycle.Rule{{ID: "off", Status: "Disabled", Expiration: lifecycle.Expiration{Days: 1}}}}
	if d, _ := ExpirationDaysFrom(disabled); d != 0 {
		t.Fatal("disabled rule counted")
	}
	scoped := &lifecycle.Configuration{Rules: []lifecycle.Rule{{ID: "s", Status: "Enabled", RuleFilter: lifecycle.Filter{Prefix: "videos/"}, Expiration: lifecycle.Expiration{Days: 3}}}}
	if _, err := ExpirationDaysFrom(scoped); err == nil {
		t.Fatal("prefix-scoped expiry accepted")
	}
	conflict := &lifecycle.Configuration{Rules: []lifecycle.Rule{
		{ID: "a", Status: "Enabled", Expiration: lifecycle.Expiration{Days: 10}},
		{ID: "b", Status: "Enabled", Expiration: lifecycle.Expiration{Days: 3}},
	}}
	if _, err := ExpirationDaysFrom(conflict); err == nil {
		t.Fatal("conflicting rules accepted")
	}
}

func TestRemovePrefixGuard(t *testing.T) {
	s := &Store{}
	for _, p := range []string{"", "videos/", "videos", "other/x/", "videos/a/b/"} {
		if err := s.RemovePrefix(t.Context(), objectFor(p)); err == nil {
			t.Errorf("RemovePrefix(%q) allowed", p)
		}
	}
}
