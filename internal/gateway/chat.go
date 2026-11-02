package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/subhammahanty235/orcha-gateway/internal/inference"
)

// statusClientClosedRequest records in logs that the client went away before a
// response was ready. The client never sees it.
const statusClientClosedRequest = 499

// chatCompletionRequest declares only the fields the gateway acts on. The
// request is forwarded as the original bytes, so other fields reach the backend
// untouched.
type chatCompletionRequest struct {
	Messages []json.RawMessage `json:"messages"`
	Stream   bool              `json:"stream"`
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.maxRequestBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large",
				"request body exceeds "+strconv.FormatInt(tooLarge.Limit, 10)+" bytes")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_body", "could not read request body")
		return
	}

	var req chatCompletionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "request body is not valid JSON for this endpoint")
		return
	}
	if len(req.Messages) == 0 {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "missing_messages", "messages must be a non-empty array")
		return
	}
	if req.Stream {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "stream_not_supported", "streaming is not supported yet; omit stream or set it to false")
		return
	}

	resp, err := s.inference.ChatCompletions(r.Context(), body)
	if err != nil {
		s.writeUpstreamError(w, r, err)
		return
	}

	// Only JSON is passed through, so that clients get a consistent error shape
	// rather than whatever a failing backend or intermediary produced.
	if mediaType, _, err := mime.ParseMediaType(resp.ContentType); err != nil || mediaType != "application/json" {
		s.logger.Error("upstream returned non-JSON response", "status", resp.StatusCode, "content_type", resp.ContentType)
		s.writeError(w, http.StatusBadGateway, "upstream_error", "bad_upstream_response", "inference backend returned an invalid response")
		return
	}

	w.Header().Set("Content-Type", resp.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(resp.Body)))
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(resp.Body); err != nil {
		s.logger.Warn("write response", "error", err)
	}
}

// writeUpstreamError maps a failed upstream call to a client response. The
// underlying error stays in the logs because it can expose internal addresses.
func (s *Server) writeUpstreamError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case r.Context().Err() != nil:
		s.logger.Info("client disconnected during inference", "error", err)
		w.WriteHeader(statusClientClosedRequest)
	case errors.Is(err, context.DeadlineExceeded):
		s.logger.Error("upstream timed out", "error", err)
		s.writeError(w, http.StatusGatewayTimeout, "upstream_error", "upstream_timeout", "inference backend timed out")
	case errors.Is(err, inference.ErrResponseTooLarge):
		s.logger.Error("upstream response too large", "error", err)
		s.writeError(w, http.StatusBadGateway, "upstream_error", "bad_upstream_response", "inference backend returned an invalid response")
	default:
		s.logger.Error("upstream request failed", "error", err)
		s.writeError(w, http.StatusBadGateway, "upstream_error", "upstream_unavailable", "inference backend is unavailable")
	}
}

// writeError writes an error in the OpenAI response shape.
func (s *Server) writeError(w http.ResponseWriter, status int, errType, code, message string) {
	s.writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    errType,
			"code":    code,
		},
	})
}
