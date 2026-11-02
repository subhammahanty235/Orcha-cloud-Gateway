package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subhammahanty235/orcha-gateway/internal/inference"
)

const validChatRequest = `{"model":"qwen","messages":[{"role":"user","content":"hi"}],"temperature":0.2,"extra":{"a":1}}`

// newTestGateway returns a gateway handler that forwards to upstream.
func newTestGateway(t *testing.T, upstream http.HandlerFunc, timeout time.Duration, maxRequestBytes int64) http.Handler {
	t.Helper()
	backend := httptest.NewServer(upstream)
	t.Cleanup(backend.Close)
	return New(testLogger(), inference.NewClient(backend.URL, timeout), maxRequestBytes).Handler()
}

func postChat(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("error body %q is not JSON: %v", rec.Body.String(), err)
	}
	if resp.Error.Message == "" || resp.Error.Type == "" {
		t.Errorf("incomplete error body: %s", rec.Body.String())
	}
	return resp.Error.Code
}

func TestChatCompletionsProxiesRequestAndResponse(t *testing.T) {
	var forwarded string
	h := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		forwarded = string(body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion"}`))
	}, time.Second, 1<<20)

	rec := postChat(h, validChatRequest)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if forwarded != validChatRequest {
		t.Errorf("forwarded body = %s, want the original bytes", forwarded)
	}
	if got := rec.Body.String(); got != `{"id":"chatcmpl-1","object":"chat.completion"}` {
		t.Errorf("body = %s", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestChatCompletionsDoesNotForwardClientHeaders(t *testing.T) {
	h := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization forwarded upstream: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}, time.Second, 1<<20)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(validChatRequest))
	req.Header.Set("Authorization", "Bearer secret-token")
	h.ServeHTTP(httptest.NewRecorder(), req)
}

func TestChatCompletionsRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"malformed json", `{"messages":`, http.StatusBadRequest, "invalid_json"},
		{"trailing data", validChatRequest + `{}`, http.StatusBadRequest, "invalid_json"},
		{"not an object", `[1,2]`, http.StatusBadRequest, "invalid_json"},
		{"messages wrong type", `{"messages":"hi"}`, http.StatusBadRequest, "invalid_json"},
		{"stream wrong type", `{"messages":[{}],"stream":"yes"}`, http.StatusBadRequest, "invalid_json"},
		{"missing messages", `{"model":"qwen"}`, http.StatusBadRequest, "missing_messages"},
		{"empty messages", `{"messages":[]}`, http.StatusBadRequest, "missing_messages"},
		{"streaming", `{"messages":[{}],"stream":true}`, http.StatusBadRequest, "stream_not_supported"},
		{"too large", `{"messages":[{"content":"` + strings.Repeat("x", 1024) + `"}]}`, http.StatusRequestEntityTooLarge, "request_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestGateway(t, func(http.ResponseWriter, *http.Request) {
				t.Error("invalid request reached the upstream")
			}, time.Second, 1024)

			rec := postChat(h, tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := errorCode(t, rec); got != tt.wantCode {
				t.Errorf("code = %q, want %q", got, tt.wantCode)
			}
		})
	}
}

func TestChatCompletionsPassesThroughUpstreamJSONErrors(t *testing.T) {
	const upstreamError = `{"error":{"code":503,"message":"Loading model","type":"unavailable_error"}}`
	h := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(upstreamError))
	}, time.Second, 1<<20)

	rec := postChat(h, validChatRequest)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Body.String() != upstreamError {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestChatCompletionsRejectsNonJSONUpstreamResponse(t *testing.T) {
	h := newTestGateway(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>internal detail</html>"))
	}, time.Second, 1<<20)

	rec := postChat(h, validChatRequest)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "bad_upstream_response" {
		t.Errorf("code = %q", got)
	}
	if strings.Contains(rec.Body.String(), "internal detail") {
		t.Errorf("upstream body leaked: %s", rec.Body.String())
	}
}

func TestChatCompletionsUpstreamTimeout(t *testing.T) {
	release := make(chan struct{})
	h := newTestGateway(t, func(http.ResponseWriter, *http.Request) {
		<-release
	}, 20*time.Millisecond, 1<<20)
	// Runs before the backend's cleanup, which waits for this handler to return.
	t.Cleanup(func() { close(release) })

	rec := postChat(h, validChatRequest)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "upstream_timeout" {
		t.Errorf("code = %q", got)
	}
}

func TestChatCompletionsUpstreamUnavailable(t *testing.T) {
	backend := httptest.NewServer(http.NotFoundHandler())
	url := backend.URL
	backend.Close()
	h := New(testLogger(), inference.NewClient(url, time.Second), 1<<20).Handler()

	rec := postChat(h, validChatRequest)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d", rec.Code)
	}
	if got := errorCode(t, rec); got != "upstream_unavailable" {
		t.Errorf("code = %q", got)
	}
	if strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Errorf("internal address leaked: %s", rec.Body.String())
	}
}

func TestChatCompletionsClientDisconnectCancelsUpstream(t *testing.T) {
	started := make(chan struct{})
	upstreamCancelled := make(chan struct{})
	h := newTestGateway(t, func(_ http.ResponseWriter, r *http.Request) {
		// The server only watches for disconnects once the body is consumed.
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(upstreamCancelled)
		case <-time.After(5 * time.Second):
		}
	}, 10*time.Second, 1<<20)

	gw := httptest.NewServer(h)
	defer gw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gw.URL+"/v1/chat/completions", strings.NewReader(validChatRequest))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err := http.DefaultClient.Do(req); !errors.Is(err, context.Canceled) {
		t.Errorf("client err = %v, want context canceled", err)
	}

	select {
	case <-upstreamCancelled:
	case <-time.After(5 * time.Second):
		t.Error("upstream request was not cancelled after the client disconnected")
	}
}
