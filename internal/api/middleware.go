package api

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// jsonResponseEnvelope is the standard response format for all API endpoints.
type jsonResponseEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Status  int         `json:"status"`
}

// responseWriter wraps http.ResponseWriter to capture the body and status code.
type responseWriter struct {
	writer   http.ResponseWriter
	status   int
	body     bytes.Buffer
	captured bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{writer: w, status: http.StatusOK}
}

func (rw *responseWriter) Header() http.Header {
	return rw.writer.Header()
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	rw.captured = true
	return rw.writer.Write(b)
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.writer.WriteHeader(statusCode)
	rw.captured = true
}

// JSONEnvelope wraps an HTTP handler with a consistent JSON response envelope.
// Every response is wrapped in {"success": bool, "data": ..., "error": ..., "status": int}.
// Exceptions:
//   - Responses with Content-Type other than application/json are passed through unchanged
//     (allows YAML, plain text, etc. to bypass the envelope)
//   - 204 No Content responses are passed through unchanged (no body to envelope)
func JSONEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		// Skip envelope for non-JSON content types and 204 No Content
		ct := rw.writer.Header().Get("Content-Type")
		if ct != "" && ct != "application/json" {
			// Already set a non-JSON content type — pass through unchanged
			return
		}
		if rw.status == http.StatusNoContent {
			// 204 has no body — nothing to envelope
			return
		}

		// Build the envelope
		var data interface{}
		var errMsg string

		if rw.body.Len() > 0 {
			// Try to parse the body as JSON already — if so, use it as data
			var parsed interface{}
			if err := json.Unmarshal(rw.body.Bytes(), &parsed); err == nil {
				data = parsed
			} else {
				// Not JSON — wrap as a string
				data = rw.body.String()
			}
		}

		// Determine success based on status code
		success := rw.status >= 200 && rw.status < 300

		envelope := jsonResponseEnvelope{
			Success: success,
			Data:    data,
			Status:  rw.status,
		}

		if !success {
			// For error responses, put the body content as the error message
			if rw.body.Len() > 0 {
				errMsg = rw.body.String()
				// Strip trailing newline if present
				envelope.Error = errMsg
			} else {
				envelope.Error = http.StatusText(rw.status)
			}
		}

		// Write the envelope
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rw.status)
		json.NewEncoder(w).Encode(envelope)
	})
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// WriteError writes a JSON error response with the given status code and message.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
