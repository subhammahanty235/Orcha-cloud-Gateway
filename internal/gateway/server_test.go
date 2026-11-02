package gateway

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subhammahanty235/orcha-gateway/internal/inference"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newHealthServer returns a Server with an unreachable backend, because /health
// has to report healthy without one.
func newHealthServer(logger *slog.Logger) *Server {
	return New(logger, inference.NewClient("http://127.0.0.1:1", time.Second), 1<<20)
}

func TestHealth(t *testing.T) {
	srv := newHealthServer(testLogger())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %s", got)
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	srv := newHealthServer(testLogger())

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestLogRequestsOmitsCredentials(t *testing.T) {
	var logs bytes.Buffer
	srv := newHealthServer(slog.New(slog.NewTextHandler(&logs, nil)))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(logs.String(), "path=/health") {
		t.Errorf("request not logged: %s", logs.String())
	}
	if strings.Contains(logs.String(), "secret-token") {
		t.Errorf("credential leaked to logs: %s", logs.String())
	}
}
