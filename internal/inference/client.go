// Package inference talks to the inference backend over its HTTP API.
package inference

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	chatCompletionsPath = "/v1/chat/completions"

	// maxResponseBytes bounds how much of a response is buffered in memory.
	// A completion from the current 2048-token context fits well within it.
	maxResponseBytes = 8 << 20 // 8 MiB

	dialTimeout = 5 * time.Second
)

// ErrResponseTooLarge reports an upstream response over maxResponseBytes.
var ErrResponseTooLarge = errors.New("inference: upstream response too large")

// Client is an HTTP client for the inference backend. It is safe for concurrent
// use. It places no limit on concurrent calls; callers own that policy.
type Client struct {
	baseURL string
	timeout time.Duration
	http    *http.Client
}

// NewClient returns a Client for the backend at baseURL. Each call is limited
// to timeout, including reading the whole response.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		timeout: timeout,
		http: &http.Client{
			// Proxy is left unset so that proxy environment variables cannot
			// reroute prompts away from the configured backend.
			Transport: &http.Transport{
				DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Response is an upstream response, buffered in full.
type Response struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

// ChatCompletions sends body, a JSON chat completion request, to the backend.
// Any HTTP status from the backend is returned as a Response; an error means no
// complete response was received. Cancelling ctx cancels the upstream request.
func (c *Client) ChatCompletions(ctx context.Context, body []byte) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("inference: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("inference: send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("inference: read response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}

	return &Response{
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        data,
	}, nil
}
