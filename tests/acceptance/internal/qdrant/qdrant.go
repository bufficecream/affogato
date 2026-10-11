// Package qdrant is a minimal Qdrant REST client for assertions (point lookup, counting,
// alias resolution). REST keeps the runner free of gRPC dependencies.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base   *url.URL
	apiKey string
	http   *http.Client
}

func New(baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("qdrant URL %q: want absolute URL", baseURL)
	}
	return &Client{base: u, apiKey: apiKey, http: &http.Client{Timeout: timeout}}, nil
}

// Point is a stored point with its payload.
type Point struct {
	ID      string         `json:"id"`
	Payload map[string]any `json:"payload"`
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	u := c.base.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("qdrant %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("qdrant %s %s: decode: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// GetPoint returns the point or found=false. collection may be an alias.
func (c *Client) GetPoint(ctx context.Context, collection, id string) (Point, bool, error) {
	var out struct {
		Result *Point `json:"result"`
	}
	code, err := c.do(ctx, http.MethodGet, "/collections/"+url.PathEscape(collection)+"/points/"+url.PathEscape(id), nil, &out)
	if err != nil || code == http.StatusNotFound || out.Result == nil {
		return Point{}, false, err
	}
	return *out.Result, true, nil
}

// CountByVideo counts points whose payload video_id matches (exact count).
func (c *Client) CountByVideo(ctx context.Context, collection, videoID string) (int, error) {
	body := map[string]any{
		"exact":  true,
		"filter": map[string]any{"must": []any{map[string]any{"key": "video_id", "match": map[string]any{"value": videoID}}}},
	}
	var out struct {
		Result struct {
			Count int `json:"count"`
		} `json:"result"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/collections/"+url.PathEscape(collection)+"/points/count", body, &out); err != nil {
		return 0, err
	}
	return out.Result.Count, nil
}

// AliasTarget returns the physical collection an alias points at ("" if none).
func (c *Client) AliasTarget(ctx context.Context, alias string) (string, error) {
	var out struct {
		Result struct {
			Aliases []struct {
				Alias      string `json:"alias_name"`
				Collection string `json:"collection_name"`
			} `json:"aliases"`
		} `json:"result"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/aliases", nil, &out); err != nil {
		return "", err
	}
	for _, a := range out.Result.Aliases {
		if a.Alias == alias {
			return a.Collection, nil
		}
	}
	return "", nil
}

// PayloadInt reads an integer payload field (JSON numbers decode as float64).
func PayloadInt(p Point, key string) (int64, bool) {
	switch v := p.Payload[key].(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	}
	return 0, false
}

// PayloadString reads a string payload field.
func PayloadString(p Point, key string) (string, bool) {
	s, ok := p.Payload[key].(string)
	return s, ok
}
