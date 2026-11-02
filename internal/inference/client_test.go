package inference

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatCompletionsForwardsRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != chatCompletionsPath {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if string(body) != `{"messages":[]}` {
			t.Errorf("body = %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1"}`))
	}))
	defer upstream.Close()

	// A trailing slash in the configured URL must not produce a double slash.
	resp, err := NewClient(upstream.URL+"/", time.Second).ChatCompletions(context.Background(), []byte(`{"messages":[]}`))
	if err != nil {
		t.Fatalf("ChatCompletions: %v", err)
	}
	if resp.StatusCode != http.StatusCreated || resp.ContentType != "application/json" || string(resp.Body) != `{"id":"1"}` {
		t.Errorf("response = %d %q %s", resp.StatusCode, resp.ContentType, resp.Body)
	}
}

func TestChatCompletionsReturnsUpstreamErrorStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "loading", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	resp, err := NewClient(upstream.URL, time.Second).ChatCompletions(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("ChatCompletions: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestChatCompletionsTimeout(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer upstream.Close()
	defer close(release)

	_, err := NewClient(upstream.URL, 20*time.Millisecond).ChatCompletions(context.Background(), []byte(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want deadline exceeded", err)
	}
}

func TestChatCompletionsResponseSizeLimit(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr error
	}{
		{"at limit", maxResponseBytes, nil},
		{"over limit", maxResponseBytes + 1, ErrResponseTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, strings.Repeat("x", tt.size))
			}))
			defer upstream.Close()

			_, err := NewClient(upstream.URL, 5*time.Second).ChatCompletions(context.Background(), []byte(`{}`))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestChatCompletionsUnreachableUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.NotFoundHandler())
	url := upstream.URL
	upstream.Close()

	if _, err := NewClient(url, time.Second).ChatCompletions(context.Background(), []byte(`{}`)); err == nil {
		t.Error("expected an error from a closed upstream")
	}
}
