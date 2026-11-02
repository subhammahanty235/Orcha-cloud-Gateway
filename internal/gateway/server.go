// Package gateway implements the client-facing HTTP API.
package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/subhammahanty235/orcha-gateway/internal/inference"
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	logger          *slog.Logger
	inference       *inference.Client
	maxRequestBytes int64
}

// New returns a Server that forwards inference requests to client and rejects
// request bodies larger than maxRequestBytes.
func New(logger *slog.Logger, client *inference.Client, maxRequestBytes int64) *Server {
	return &Server{logger: logger, inference: client, maxRequestBytes: maxRequestBytes}
}

// Handler returns the gateway's routes wrapped in request logging.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	return s.logRequests(mux)
}

// handleHealth is a liveness check: it reports that the gateway process can
// serve requests and deliberately does not contact the inference backend.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Warn("write response", "error", err)
	}
}

// logRequests records one line per request. It logs only the method, path,
// status, and duration: never headers or bodies, which can hold credentials
// and prompts.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
