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

// isODataEnvelope detects whether a JSON body is already an OData-style
// envelope (i.e. has both "data" and "meta" top-level keys). When
// detected, JSONEnvelope passes the body through unchanged to avoid
// double-wrapping handler-level OData responses.
func isODataEnvelope(body []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	_, hasData := raw["data"]
	_, hasMeta := raw["meta"]
	return hasData && hasMeta
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
	return len(b), nil
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.captured = true
}

// flushCaptured writes the captured body and status code to the underlying
// ResponseWriter. This is used when the middleware decides to pass the
// response through unchanged (e.g. successful 2xx, non-JSON, 204).
func (rw *responseWriter) flushCaptured() {
	rw.writer.WriteHeader(rw.status)
	rw.writer.Write(rw.body.Bytes())
}

// JSONEnvelope wraps an HTTP handler with a consistent JSON response envelope.
// Successful 2xx responses pass through unchanged (raw JSON body only).
// Error responses (4xx/5xx) are wrapped in {"success":false,"error":"...","status":N}.
// Exceptions:
//   - Responses with Content-Type other than application/json are passed through unchanged
//     (allows YAML, plain text, etc. to bypass the envelope)
//   - 204 No Content responses are passed through unchanged (no body to envelope)
//   - OData-style responses (with "data" + "meta" keys) pass through unchanged
func JSONEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := newResponseWriter(w)
		next.ServeHTTP(rw, r)

		// Skip envelope for non-JSON content types
		ct := rw.writer.Header().Get("Content-Type")
		if ct != "" && ct != "application/json" {
			rw.flushCaptured()
			return
		}
		if rw.status == http.StatusNoContent {
			// 204 has no body — pass through status only
			rw.writer.WriteHeader(http.StatusNoContent)
			return
		}

		// Successful 2xx responses pass through unchanged — raw body only.
		if rw.status >= 200 && rw.status < 300 {
			// Check for OData-style response (has "data" + "meta" keys)
			if rw.body.Len() > 0 && isODataEnvelope(rw.body.Bytes()) {
				rw.flushCaptured()
				return
			}
			// Pass through raw JSON for success responses
			rw.flushCaptured()
			return
		}

		// Build the error envelope for 4xx/5xx responses
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

		envelope := jsonResponseEnvelope{
			Success: false,
			Data:    data,
			Status:  rw.status,
		}

		if rw.body.Len() > 0 {
			errMsg = rw.body.String()
			envelope.Error = errMsg
		} else {
			envelope.Error = http.StatusText(rw.status)
		}

		// Write the error envelope
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
