package httpclient

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PresignCheck describes what a presigned MinIO GET URL must satisfy (spec 2.1, A5, B2).
type PresignCheck struct {
	PublicEndpoint string        // AF_MINIO_PUBLIC_ENDPOINT, e.g. http://minio.localtest.me:9000
	Bucket, Key    string        // expected object (path-style addressing)
	MaxTTL         time.Duration // 10 min per contract
	Now            time.Time     // reference time; zero = time.Now()
	ClockSkew      time.Duration // tolerated skew between runner and API clocks
}

// CheckPresignedURL validates a SigV4 presigned URL: host and scheme of the public endpoint,
// path-style bucket/key, signed GET parameters, X-Amz-Expires <= MaxTTL, and a signing time
// that makes the URL expire within MaxTTL of now.
func CheckPresignedURL(u *url.URL, c PresignCheck) error {
	pub, err := url.Parse(c.PublicEndpoint)
	if err != nil || pub.Host == "" {
		return fmt.Errorf("public endpoint %q invalid", c.PublicEndpoint)
	}
	if !strings.EqualFold(u.Scheme, pub.Scheme) || !strings.EqualFold(u.Host, pub.Host) {
		return fmt.Errorf("presigned URL %s://%s is not on public endpoint %s://%s", u.Scheme, u.Host, pub.Scheme, pub.Host)
	}
	wantPath := strings.TrimSuffix(pub.Path, "/") + "/" + c.Bucket + "/" + c.Key
	if u.Path != wantPath {
		return fmt.Errorf("presigned path %q, want %q", u.Path, wantPath)
	}
	q := u.Query()
	for _, p := range []string{"X-Amz-Algorithm", "X-Amz-Credential", "X-Amz-Date", "X-Amz-Expires", "X-Amz-SignedHeaders", "X-Amz-Signature"} {
		if q.Get(p) == "" {
			return fmt.Errorf("presigned URL missing %s", p)
		}
	}
	if q.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
		return fmt.Errorf("unexpected X-Amz-Algorithm %q", q.Get("X-Amz-Algorithm"))
	}
	expires, err := strconv.Atoi(q.Get("X-Amz-Expires"))
	if err != nil || expires <= 0 {
		return fmt.Errorf("bad X-Amz-Expires %q", q.Get("X-Amz-Expires"))
	}
	ttl := time.Duration(expires) * time.Second
	if ttl > c.MaxTTL {
		return fmt.Errorf("presigned TTL %s exceeds %s", ttl, c.MaxTTL)
	}
	signed, err := time.Parse("20060102T150405Z", q.Get("X-Amz-Date"))
	if err != nil {
		return fmt.Errorf("bad X-Amz-Date %q: %w", q.Get("X-Amz-Date"), err)
	}
	now := c.Now
	if now.IsZero() {
		now = time.Now()
	}
	expiry := signed.Add(ttl)
	if expiry.After(now.Add(c.MaxTTL + c.ClockSkew)) {
		return fmt.Errorf("URL expires at %s, later than now+%s", expiry.Format(time.RFC3339), c.MaxTTL)
	}
	if !expiry.After(now.Add(-c.ClockSkew)) {
		return fmt.Errorf("URL already expired at %s", expiry.Format(time.RFC3339))
	}
	// Presigned GETs must not grant anything beyond the object (e.g. list or upload params).
	for _, forbidden := range []string{"uploadId", "partNumber", "list-type", "prefix"} {
		if q.Has(forbidden) {
			return fmt.Errorf("presigned URL carries unexpected parameter %q", forbidden)
		}
	}
	return nil
}
