// Package s3assert wraps minio-go with the object assertions acceptance steps need.
package s3assert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/lifecycle"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("object not found")

type Store struct {
	c       *minio.Client
	maxRead int64
}

// New connects with path-style addressing (MinIO, B1 decision 11).
func New(endpoint, accessKey, secretKey, region string, useTLS bool, maxRead int64) (*Store, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       useTLS,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, err
	}
	return &Store{c: c, maxRead: maxRead}, nil
}

// Info is the subset of object metadata steps assert on.
type Info struct {
	Size         int64
	ContentType  string
	CacheControl string
	ETag         string
	UserMetadata map[string]string // x-amz-meta-* without prefix, lower-case keys
}

func (s *Store) Stat(ctx context.Context, o contracts.Object) (Info, error) {
	st, err := s.c.StatObject(ctx, o.Bucket, o.Key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return Info{}, fmt.Errorf("%s: %w", o, ErrNotFound)
		}
		return Info{}, fmt.Errorf("stat %s: %w", o, err)
	}
	meta := map[string]string{}
	for k, v := range st.UserMetadata {
		meta[strings.ToLower(k)] = v
	}
	return Info{Size: st.Size, ContentType: st.ContentType, CacheControl: st.Metadata.Get("Cache-Control"), ETag: st.ETag, UserMetadata: meta}, nil
}

// AssertExists checks the object exists, is non-empty and (if given) has the content type.
func (s *Store) AssertExists(ctx context.Context, o contracts.Object, wantContentType string) (Info, error) {
	info, err := s.Stat(ctx, o)
	if err != nil {
		return info, err
	}
	if info.Size == 0 {
		return info, fmt.Errorf("%s exists but is empty", o)
	}
	if wantContentType != "" {
		if err := SameMediaType(info.ContentType, wantContentType); err != nil {
			return info, fmt.Errorf("%s: %w", o, err)
		}
	}
	return info, nil
}

// AssertAbsent checks that the object does not exist.
func (s *Store) AssertAbsent(ctx context.Context, o contracts.Object) error {
	_, err := s.Stat(ctx, o)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("%s exists, want absent", o)
	}
}

// Get reads an object fully, bounded by maxRead.
func (s *Store) Get(ctx context.Context, o contracts.Object) ([]byte, error) {
	obj, err := s.c.GetObject(ctx, o.Bucket, o.Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", o, err)
	}
	defer obj.Close()
	b, err := io.ReadAll(io.LimitReader(obj, s.maxRead+1))
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, fmt.Errorf("%s: %w", o, ErrNotFound)
		}
		return nil, fmt.Errorf("read %s: %w", o, err)
	}
	if int64(len(b)) > s.maxRead {
		return nil, fmt.Errorf("%s larger than %d bytes", o, s.maxRead)
	}
	return b, nil
}

// AssertBytesEqual compares an object's bytes to want.
func (s *Store) AssertBytesEqual(ctx context.Context, o contracts.Object, want []byte) error {
	got, err := s.Get(ctx, o)
	if err != nil {
		return err
	}
	if err := CompareBytes(want, got); err != nil {
		return fmt.Errorf("%s: %w", o, err)
	}
	return nil
}

// Put uploads test input (e.g. a raw chunk with corrupted bytes) with the contract metadata.
func (s *Store) Put(ctx context.Context, o contracts.Object, body []byte, contentType string, meta map[string]string) (sha256Hex string, err error) {
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	_, err = s.c.PutObject(ctx, o.Bucket, o.Key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: meta,
	})
	if err != nil {
		return "", fmt.Errorf("put %s: %w", o, err)
	}
	return sha, nil
}

// Remove deletes one object (cleanup / "thumbnail does not exist yet" setup).
func (s *Store) Remove(ctx context.Context, o contracts.Object) error {
	return s.c.RemoveObject(ctx, o.Bucket, o.Key, minio.RemoveObjectOptions{})
}

// RemovePrefix deletes every object under prefix (per-video cleanup).
func (s *Store) RemovePrefix(ctx context.Context, prefix contracts.Object) error {
	if !strings.HasPrefix(prefix.Key, "videos/") || !strings.HasSuffix(prefix.Key, "/") || strings.Count(prefix.Key, "/") != 2 {
		return fmt.Errorf("refusing to remove prefix %q: only videos/{id}/ prefixes", prefix)
	}
	objs := s.c.ListObjects(ctx, prefix.Bucket, minio.ListObjectsOptions{Prefix: prefix.Key, Recursive: true})
	var errs []error
	for e := range s.c.RemoveObjects(ctx, prefix.Bucket, objs, minio.RemoveObjectsOptions{}) {
		errs = append(errs, e.Err)
	}
	return errors.Join(errs...)
}

// ExpirationDays returns the enabled whole-bucket expiration rule in days (0 = none).
func (s *Store) ExpirationDays(ctx context.Context, bucket string) (int, error) {
	cfg, err := s.c.GetBucketLifecycle(ctx, bucket)
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchLifecycleConfiguration" {
			return 0, nil
		}
		return 0, err
	}
	return ExpirationDaysFrom(cfg)
}

// ExpirationDaysFrom extracts the object-expiry days from a lifecycle config. Rules that
// only abort multipart uploads are ignored; prefix-scoped expiry rules are an error because
// the contract expects bucket-wide expiry (or none).
func ExpirationDaysFrom(cfg *lifecycle.Configuration) (int, error) {
	if cfg == nil {
		return 0, nil
	}
	days := 0
	for _, r := range cfg.Rules {
		if r.Status != "Enabled" || r.Expiration.IsNull() {
			continue
		}
		if r.RuleFilter.Prefix != "" || r.RuleFilter.And.Prefix != "" || r.Prefix != "" {
			return 0, fmt.Errorf("rule %q: expiry scoped to a prefix", r.ID)
		}
		if d := int(r.Expiration.Days); d > 0 {
			if days != 0 && d != days {
				return 0, fmt.Errorf("conflicting expiry rules: %d and %d days", days, d)
			}
			days = d
		} else {
			return 0, fmt.Errorf("rule %q: expiry by date is not allowed", r.ID)
		}
	}
	return days, nil
}

// SameMediaType compares two Content-Type values, ignoring parameters and case.
func SameMediaType(got, want string) error {
	g, _, err1 := mime.ParseMediaType(got)
	w, _, err2 := mime.ParseMediaType(want)
	if err1 != nil || err2 != nil || !strings.EqualFold(g, w) {
		return fmt.Errorf("content-type %q, want %q", got, want)
	}
	return nil
}

// CompareBytes reports the first differing offset with a short hex context.
func CompareBytes(want, got []byte) error {
	if bytes.Equal(want, got) {
		return nil
	}
	n := min(len(want), len(got))
	off := n
	for i := 0; i < n; i++ {
		if want[i] != got[i] {
			off = i
			break
		}
	}
	ctx := func(b []byte) string {
		end := min(len(b), off+8)
		if off >= len(b) {
			return "<eof>"
		}
		return hex.EncodeToString(b[off:end])
	}
	return fmt.Errorf("bytes differ at offset %d (want len %d, got len %d): want %s… got %s…",
		off, len(want), len(got), ctx(want), ctx(got))
}
