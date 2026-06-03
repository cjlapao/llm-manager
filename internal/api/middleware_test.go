package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsODataEnvelope(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		odata bool
	}{
		{
			name:  "odatalistresponse with data array and meta",
			body:  `{"data":[{"slug":"test"}],"meta":{"total":1,"page":1,"limit":25,"total_pages":1}}`,
			odata: true,
		},
		{
			name:  "odatalistresponse with data object and meta",
			body:  `{"data":{"embed_models":[],"rerank_models":[]},"meta":{"total":0,"page":1,"limit":25,"total_pages":0}}`,
			odata: true,
		},
		{
			name:  "only data key, no meta",
			body:  `{"data":[{"slug":"test"}]}`,
			odata: false,
		},
		{
			name:  "only meta key, no data",
			body:  `{"meta":{"total":1}}`,
			odata: false,
		},
		{
			name:  "json response envelope",
			body:  `{"success":true,"data":[{"slug":"test"}],"status":200}`,
			odata: false,
		},
		{
			name:  "plain json array",
			body:  `[{"slug":"test"}]`,
			odata: false,
		},
		{
			name:  "empty object",
			body:  `{}`,
			odata: false,
		},
		{
			name:  "invalid json",
			body:  `not json`,
			odata: false,
		},
		{
			name:  "error response envelope",
			body:  `{"success":false,"error":"bad request","status":400}`,
			odata: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isODataEnvelope([]byte(tc.body))
			if result != tc.odata {
				t.Errorf("isODataEnvelope(%q) = %v, want %v", tc.body, result, tc.odata)
			}
		})
	}
}

func TestJSONEnvelopeSkipsODataResponse(t *testing.T) {
	// Handler that writes an ODataListResponse
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"slug":"test"}],"meta":{"total":1,"page":1,"limit":25,"total_pages":1}}`))
	})

	mw := JSONEnvelope(handler)
	req := httptest.NewRequest(http.MethodGet, "/api/models?page=1&limit=25", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	body := rec.Body.String()

	// OData response should pass through unchanged — no "success" key
	if strings.Contains(body, `"success"`) {
		t.Errorf("OData response was double-wrapped with success envelope: %s", body)
	}
	// Should contain the original data and meta keys
	if !strings.Contains(body, `"data"`) {
		t.Error("OData response missing 'data' key after middleware")
	}
	if !strings.Contains(body, `"meta"`) {
		t.Error("OData response missing 'meta' key after middleware")
	}
}

func TestJSONEnvelopeWrapsNormalResponse(t *testing.T) {
	// Handler that writes a normal JSON array (no OData params)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"slug":"test"}]`))
	})

	mw := JSONEnvelope(handler)
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	body := rec.Body.String()

	// Successful response passes through unchanged — raw body only, no envelope
	if strings.Contains(body, `"success"`) {
		t.Errorf("Successful response was unexpectedly wrapped: %s", body)
	}
	if body != `[{"slug":"test"}]` {
		t.Errorf("Normal response body mismatch: %s", body)
	}
}

func TestJSONEnvelopeWrapsErrorResponse(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	})

	mw := JSONEnvelope(handler)
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	body := rec.Body.String()

	if !strings.Contains(body, `"success":false`) {
		t.Errorf("Error response was not wrapped with success:false: %s", body)
	}
	if !strings.Contains(body, `"error"`) {
		t.Errorf("Error response missing error field: %s", body)
	}
}

func TestJSONEnvelopeSkips204(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mw := JSONEnvelope(handler)
	req := httptest.NewRequest(http.MethodDelete, "/api/models/test", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("204 was modified: status=%d, body=%q", rec.Code, rec.Body.String())
	}
}

func TestJSONEnvelopeSkipsNonJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	})

	mw := JSONEnvelope(handler)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	body := rec.Body.String()
	if body != "hello" {
		t.Errorf("Non-JSON response was wrapped: %s", body)
	}
}
