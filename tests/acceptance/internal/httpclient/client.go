// Package httpclient is the Serving API client for acceptance tests. It never follows
// redirects, so steps can assert 302 + Location, 409 and 410 exactly as returned.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrBodyTooLarge is returned when a response exceeds the configured cap.
var ErrBodyTooLarge = errors.New("response body exceeds limit")

type Client struct {
	base    *url.URL
	http    *http.Client
	maxBody int64
}

// Response is a fully buffered response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	URL        string
	Elapsed    time.Duration
}

// New builds a client for baseURL. maxBody caps buffered bodies (DoS guard for the runner).
func New(baseURL string, timeout time.Duration, maxBody int64) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("base URL %q: want absolute URL", baseURL)
	}
	if maxBody <= 0 {
		return nil, fmt.Errorf("maxBody must be positive")
	}
	return &Client{
		base: u,
		http: &http.Client{
			Timeout: timeout,
			// Return the 3xx response itself instead of following it.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		maxBody: maxBody,
	}, nil
}

// Resolve turns an API path (or absolute URL from a response) into an absolute URL.
func (c *Client) Resolve(pathOrURL string) (*url.URL, error) {
	ref, err := url.Parse(pathOrURL)
	if err != nil {
		return nil, err
	}
	return c.base.ResolveReference(ref), nil
}

// Get performs a GET against the API base URL.
func (c *Client) Get(ctx context.Context, pathOrURL string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, pathOrURL, nil)
}

// Do performs a request with optional extra headers and buffers the body up to maxBody.
func (c *Client) Do(ctx context.Context, method, pathOrURL string, header http.Header) (*Response, error) {
	u, err := c.Resolve(pathOrURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > c.maxBody {
		return nil, fmt.Errorf("%s %s: %w (%d bytes)", method, u, ErrBodyTooLarge, c.maxBody)
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: body, URL: u.String(), Elapsed: time.Since(start)}, nil
}

// ExpectStatus returns an error describing the response when the status differs.
func (r *Response) ExpectStatus(want int) error {
	if r.StatusCode != want {
		return fmt.Errorf("GET %s: status %d, want %d; body: %s", r.URL, r.StatusCode, want, truncate(r.Body, 512))
	}
	return nil
}

// ExpectHeader checks one header value exactly (after trimming).
func (r *Response) ExpectHeader(name, want string) error {
	if got := strings.TrimSpace(r.Header.Get(name)); got != want {
		return fmt.Errorf("GET %s: header %s = %q, want %q", r.URL, name, got, want)
	}
	return nil
}

// ExpectMediaType checks Content-Type ignoring parameters such as charset.
func (r *Response) ExpectMediaType(want string) error {
	got := r.Header.Get("Content-Type")
	mt, _, _ := strings.Cut(got, ";")
	if !strings.EqualFold(strings.TrimSpace(mt), want) {
		return fmt.Errorf("GET %s: Content-Type %q, want %q", r.URL, got, want)
	}
	return nil
}

// Location returns the parsed redirect target, resolved against the request URL.
func (r *Response) Location() (*url.URL, error) {
	loc := r.Header.Get("Location")
	if loc == "" {
		return nil, fmt.Errorf("GET %s: no Location header (status %d)", r.URL, r.StatusCode)
	}
	base, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(loc)
	if err != nil {
		return nil, fmt.Errorf("GET %s: bad Location %q: %w", r.URL, loc, err)
	}
	return base.ResolveReference(ref), nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…(" + strconv.Itoa(len(b)) + " bytes)"
}
