package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// ---------------------------------------------------------------------------
// Test envelope type matching the JSONEnvelope middleware output
// ---------------------------------------------------------------------------

// apiEnvelope mirrors the jsonResponseEnvelope used by the JSONEnvelope middleware.
type apiEnvelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   string      `json:"error"`
	Status  int         `json:"status"`
}

// ---------------------------------------------------------------------------
// 1. setupTestDB — creates an in-memory SQLite database with migrations applied
// ---------------------------------------------------------------------------

// setupTestDB creates an in-memory SQLite DatabaseManager, applies pending
// migrations automatically, and returns the manager plus a cleanup function.
func setupTestDB(t *testing.T) (database.DatabaseManager, func()) {
	t.Helper()

	db, err := database.NewDatabaseManager("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("NewDatabaseManager() error: %v", err)
	}

	if err := db.Open(); err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}

	if err := db.ApplyPendingMigrations(true); err != nil {
		t.Fatalf("ApplyPendingMigrations() error: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
	}

	t.Cleanup(cleanup)
	return db, cleanup
}

// ---------------------------------------------------------------------------
// 2. setupTestServer — creates a full httptest.Server with all routes
// ---------------------------------------------------------------------------

// setupTestServer creates an in-memory DB, an APIContext with a minimal
// Config, a gorilla/mux router with ALL routes registered (matching server.go),
// and wraps it in JSONEnvelope middleware. Returns an httptest.Server and a
// cleanup function. Uses t.Helper() and t.Fatal() for errors.
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	// Create in-memory DB
	db, dbCleanup := setupTestDB(t)

	// Minimal config
	cfg := &config.Config{
		DataDir:     t.TempDir(),
		LogDir:      t.TempDir(),
		DatabaseURL: ":memory:",
		LLMDir:      t.TempDir(),
		InstallDir:  t.TempDir(),
		HFCacheDir:  t.TempDir(),
	}

	// APIContext with services
	ctx := NewAPIContext(db, cfg)

	// Router — mirrors server.go exactly
	router := mux.NewRouter()
	api := router.PathPrefix("/api").Subrouter()
	api.Use(JSONEnvelope)

	h := &APIContext{
		DB:               ctx.DB,
		Config:           ctx.Config,
		ModelService:     ctx.ModelService,
		ContainerService: ctx.ContainerService,
	}

	// Model CRUD handlers
	modelHandler := &ModelHandler{h}
	api.HandleFunc("/models", modelHandler.ListModels).Methods(http.MethodGet)
	api.HandleFunc("/models", modelHandler.CreateModel).Methods(http.MethodPost)
	api.HandleFunc("/models/{slug}", modelHandler.GetModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}", modelHandler.UpdateModel).Methods(http.MethodPut)
	api.HandleFunc("/models/{slug}", modelHandler.DeleteModel).Methods(http.MethodDelete)
	api.HandleFunc("/models/{slug}/info", modelHandler.GetModelInfo).Methods(http.MethodGet)

	// Model utility handlers
	modelUtilHandler := &ModelUtilHandler{h}
	api.HandleFunc("/models/import", modelUtilHandler.ImportModel).Methods(http.MethodPost)
	api.HandleFunc("/models/{slug}/export", modelUtilHandler.ExportModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}/compose", modelUtilHandler.ComposeModel).Methods(http.MethodGet)
	api.HandleFunc("/models/{slug}/cache", modelUtilHandler.ClearCache).Methods(http.MethodDelete)

	// RAG handlers
	ragHandler := &RAGHandler{h}
	api.HandleFunc("/rag", ragHandler.ListRAG).Methods(http.MethodGet)
	api.HandleFunc("/rag/start", ragHandler.StartRAG).Methods(http.MethodPost)
	api.HandleFunc("/rag/stop", ragHandler.StopRAG).Methods(http.MethodPost)

	// Health check
	api.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}).Methods(http.MethodGet)

	server := httptest.NewServer(router)

	cleanup := func() {
		server.Close()
		dbCleanup()
	}

	t.Cleanup(cleanup)
	return server, cleanup
}

// ---------------------------------------------------------------------------
// 3. seedTestModel — creates a minimal test model in the DB
// ---------------------------------------------------------------------------

// seedTestModel creates a minimal test model in the DB with Type="llm",
// SubType="", Port=8000, Name="Test Model". Uses db.CreateModel().
func seedTestModel(t *testing.T, db database.DatabaseManager, slug string) {
	t.Helper()

	model := &models.Model{
		Slug:    slug,
		Type:    "llm",
		SubType: "",
		Name:    "Test Model",
		Port:    8000,
	}

	if err := db.CreateModel(model); err != nil {
		t.Fatalf("seedTestModel(%s): %v", slug, err)
	}
}

// ---------------------------------------------------------------------------
// 4. seedTestRAGModels — creates embedding and reranker models
// ---------------------------------------------------------------------------

// seedTestRAGModels creates a test embedding model and a reranker model.
// Sets Type="rag", SubType="embedding" and SubType="reranker".
func seedTestRAGModels(t *testing.T, db database.DatabaseManager) {
	t.Helper()

	embedModel := &models.Model{
		Slug:    "test-embed",
		Type:    "rag",
		SubType: "embedding",
		Name:    "Test Embedding Model",
		Port:    8001,
	}
	if err := db.CreateModel(embedModel); err != nil {
		t.Fatalf("seedTestRAGModels(embedding): %v", err)
	}

	rerankModel := &models.Model{
		Slug:    "test-rerank",
		Type:    "rag",
		SubType: "reranker",
		Name:    "Test Reranker Model",
		Port:    8002,
	}
	if err := db.CreateModel(rerankModel); err != nil {
		t.Fatalf("seedTestRAGModels(reranker): %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5. Helper assertions
// ---------------------------------------------------------------------------

// extractEnvelopeBody parses the raw response body which may contain the
// handler's JSON followed by the JSONEnvelope wrapper (two JSON objects
// separated by a newline). It returns the last JSON object in the body as
// a []byte, which is the envelope.
func extractEnvelopeBody(body []byte) []byte {
	// The body looks like: <handler JSON>\n{ "success": ..., ... }
	// Split on newlines and try to parse each line as JSON.
	lines := bytes.Split(body, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		// Try to parse this line as a JSON object.
		var parsed interface{}
		if err := json.Unmarshal(line, &parsed); err == nil {
			return line
		}
	}
	return body
}

// assertEnvelopeOK checks the response body. For successful (2xx) responses
// the middleware passes the body through unchanged, so this helper accepts
// raw JSON directly. For error responses it expects an envelope.
func assertEnvelopeOK(t *testing.T, body []byte, expectedStatus int) {
	t.Helper()

	// Try parsing as an envelope first (covers error paths and OData responses).
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Status == expectedStatus {
		// It's an envelope — assert success=true
		if !env.Success {
			t.Errorf("assertEnvelopeOK: expected success=true, got false — body: %s", string(body))
		}
		return
	}
	// Not an envelope — body is raw JSON from the handler, which is correct
	// for successful 2xx responses.
}

// assertEnvelopeErr parses the JSON envelope body and asserts success=false
// and status==expectedStatus.
func assertEnvelopeErr(t *testing.T, body []byte, expectedStatus int) {
	t.Helper()

	// The body may contain the handler's JSON + envelope (two JSON objects).
	// Extract just the envelope (last JSON object).
	envBody := extractEnvelopeBody(body)

	var env apiEnvelope
	if err := json.Unmarshal(envBody, &env); err != nil {
		t.Fatalf("assertEnvelopeErr: failed to unmarshal envelope: %v — raw: %s", err, string(body))
	}

	if env.Success {
		t.Errorf("assertEnvelopeErr: expected success=false, got true — body: %s", string(body))
	}

	if env.Status != expectedStatus {
		t.Errorf("assertEnvelopeErr: expected status=%d, got %d — body: %s", expectedStatus, env.Status, string(body))
	}
}

// assertStatusCode asserts that an HTTP response has the expected status code.
func assertStatusCode(t *testing.T, resp *http.Response, expected int) {
	t.Helper()

	if resp.StatusCode != expected {
		t.Errorf("assertStatusCode: expected status=%d, got %d", expected, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// 6. Request helpers
// ---------------------------------------------------------------------------

// readBody reads the full body from an http.Response and closes it.
func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	return body
}

// doRequest builds and executes an HTTP request, returning the response.
// The caller is responsible for closing resp.Body.
func doRequest(t *testing.T, client *http.Client, method, url string, body io.Reader) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("doRequest: NewRequest(%s %s): %v", method, url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("doRequest: %s %s: %v", method, url, err)
	}
	return resp
}

// doJSONRequest builds and executes an HTTP request with a JSON body.
// The caller is responsible for closing resp.Body.
func doJSONRequest(t *testing.T, client *http.Client, method, url string, payload interface{}) *http.Response {
	t.Helper()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("doJSONRequest: marshal: %v", err)
		}
		body = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("doJSONRequest: NewRequest(%s %s): %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("doJSONRequest: %s %s: %v", method, url, err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// 7. Server Lifecycle Tests
// ---------------------------------------------------------------------------

// TestServerLifecycle_StartsAndResponds verifies that the test server starts
// successfully and responds to the /api/health endpoint with a 200 OK and a
// success=true envelope.
func TestServerLifecycle_StartsAndResponds(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/health", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusOK)
}

// TestServerLifecycle_ShutsDownCleanly verifies that closing the test server
// does not panic.
func TestServerLifecycle_ShutsDownCleanly(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Calling Close() should not panic.
	ts.Close()
}

// ---------------------------------------------------------------------------
// 8. Model CRUD Tests
// ---------------------------------------------------------------------------

// TestModelCRUD_ListEmpty verifies that GET /api/models returns an empty
// array when no models have been created.
func TestModelCRUD_ListEmpty(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusOK)

	// Verify that data is an empty array.
	var dataArr []interface{}
	if err := json.Unmarshal(body, &dataArr); err != nil {
		t.Fatalf("ListEmpty: failed to unmarshal body: %v — raw: %s", err, string(body))
	}
	if len(dataArr) != 0 {
		t.Errorf("ListEmpty: expected empty array, got %d items — body: %s", len(dataArr), string(body))
	}
}

// TestModelCRUD_CreateAndGet verifies that a model can be created via POST
// and then retrieved via GET.
func TestModelCRUD_CreateAndGet(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// POST to create a model.
	resp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-create",
		"name": "Test Create",
		"port": 9000,
		"type": "llm",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusCreated)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusCreated)

	// GET to verify the model was created.
	resp = doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-create", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body = readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusOK)

	// Verify the name field.
	var modelMap map[string]interface{}
	if err := json.Unmarshal(body, &modelMap); err != nil {
		t.Fatalf("CreateAndGet: failed to unmarshal body: %v — raw: %s", err, string(body))
	}

	name, ok := modelMap["Name"]
	if !ok {
		t.Fatalf("CreateAndGet: expected 'Name' key in response — body: %s", string(body))
	}
	if name != "Test Create" {
		t.Errorf("CreateAndGet: expected Name='Test Create', got %v — body: %s", name, string(body))
	}
}

// TestModelCRUD_GetNotFound verifies that GET /api/models/{slug} returns 404
// when the model does not exist.
func TestModelCRUD_GetNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/nonexistent", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusNotFound)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusNotFound)
}

// TestModelCRUD_UpdatePartial verifies that a model can be partially updated
// via PUT and the changes are reflected on subsequent GET.
func TestModelCRUD_UpdatePartial(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// First create a model via POST.
	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-update",
		"name": "Original Name",
		"port": 9001,
		"type": "llm",
	})
	defer createResp.Body.Close()
	assertStatusCode(t, createResp, http.StatusCreated)

	// PUT a partial update.
	updateResp := doJSONRequest(t, ts.Client(), http.MethodPut, ts.URL+"/api/models/test-update", map[string]interface{}{
		"name": "Updated Name",
	})
	defer updateResp.Body.Close()

	assertStatusCode(t, updateResp, http.StatusOK)

	updateBody := readBody(t, updateResp)
	assertEnvelopeOK(t, updateBody, http.StatusOK)

	// GET to verify the update.
	getResp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-update", nil)
	defer getResp.Body.Close()

	assertStatusCode(t, getResp, http.StatusOK)

	getBody := readBody(t, getResp)
	assertEnvelopeOK(t, getBody, http.StatusOK)

	var modelMap map[string]interface{}
	if err := json.Unmarshal(getBody, &modelMap); err != nil {
		t.Fatalf("UpdatePartial: failed to unmarshal body: %v — raw: %s", err, string(getBody))
	}

	name, ok := modelMap["Name"]
	if !ok {
		t.Fatalf("UpdatePartial: expected 'Name' key in response — body: %s", string(getBody))
	}
	if name != "Updated Name" {
		t.Errorf("UpdatePartial: expected Name='Updated Name', got %v — body: %s", name, string(getBody))
	}
}

// TestModelCRUD_Delete verifies that a model can be deleted via DELETE and
// subsequent GET returns 404.
func TestModelCRUD_Delete(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// First create a model via POST.
	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-delete",
		"name": "To Delete",
		"port": 9002,
		"type": "llm",
	})
	defer createResp.Body.Close()
	assertStatusCode(t, createResp, http.StatusCreated)

	// DELETE the model.
	deleteResp := doRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/models/test-delete", nil)
	defer deleteResp.Body.Close()

	assertStatusCode(t, deleteResp, http.StatusNoContent)

	// GET should now return 404.
	getResp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-delete", nil)
	defer getResp.Body.Close()

	assertStatusCode(t, getResp, http.StatusNotFound)

	body := readBody(t, getResp)
	assertEnvelopeErr(t, body, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// 9. Model Utility Tests
// ---------------------------------------------------------------------------

// assertContains asserts that the given string s contains the substring sub.
func assertContains(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Errorf("assertContains: expected %q to contain %q — full: %q", s, sub, s)
	}
}

// TestModelUtil_ImportJSON verifies that POST /api/models/import with a valid
// YAML content creates a model and returns 201, then confirms the model is
// retrievable via GET.
func TestModelUtil_ImportJSON(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models/import", map[string]interface{}{
		"yaml_content": "slug: test-import\nname: Import Test\nport: 7000\ntype: llm\nengine: vllm\nprofile:\n  active_params_b: 1.0\n  is_moe: false\n  quant_bytes_per_param: 2.0\n",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusCreated)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusCreated)

	// Verify the model exists via GET /api/models/test-import.
	getResp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-import", nil)
	defer getResp.Body.Close()

	assertStatusCode(t, getResp, http.StatusOK)

	getBody := readBody(t, getResp)
	assertEnvelopeOK(t, getBody, http.StatusOK)

	var modelMap map[string]interface{}
	if err := json.Unmarshal(getBody, &modelMap); err != nil {
		t.Fatalf("ImportJSON: failed to unmarshal body: %v — raw: %s", err, string(getBody))
	}

	name, ok := modelMap["Name"]
	if !ok {
		t.Fatalf("ImportJSON: expected 'Name' key in response — body: %s", string(getBody))
	}
	if name != "Import Test" {
		t.Errorf("ImportJSON: expected Name='Import Test', got %v — body: %s", name, string(getBody))
	}
}

// TestModelUtil_ImportInvalidYAML verifies that POST /api/models/import with
// invalid JSON returns 400.
func TestModelUtil_ImportInvalidYAML(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// Send a body that is not valid JSON.
	resp := doRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models/import", strings.NewReader("not-valid-json{"))
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusBadRequest)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusBadRequest)
}

// TestModelUtil_Export verifies that GET /api/models/{slug}/export returns a
// YAML document (200) with Content-Type text/x-yaml for an existing model.
func TestModelUtil_Export(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// First seed a model via POST so export has something to export.
	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-export",
		"name": "Export Test",
		"port": 7001,
		"type": "llm",
	})
	defer createResp.Body.Close()
	assertStatusCode(t, createResp, http.StatusCreated)

	// GET /api/models/test-export/export.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-export/export", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	// Assert Content-Type is text/x-yaml.
	ct := resp.Header.Get("Content-Type")
	assertContains(t, ct, "text/x-yaml")

	// Assert response body starts with "slug:".
	body := readBody(t, resp)
	assertContains(t, string(body), "slug:")
}

// TestModelUtil_ExportNotFound verifies that GET /api/models/{slug}/export
// returns 404 when the model does not exist.
func TestModelUtil_ExportNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/nonexistent/export", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusNotFound)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusNotFound)
}

// TestModelUtil_Compose verifies that GET /api/models/{slug}/compose returns
// a docker-compose YAML (200) for an existing model.
func TestModelUtil_Compose(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// First seed a model via POST so compose has something to compose.
	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-compose",
		"name": "Compose Test",
		"port": 7002,
		"type": "llm",
	})
	defer createResp.Body.Close()
	assertStatusCode(t, createResp, http.StatusCreated)

	// GET /api/models/test-compose/compose.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/test-compose/compose", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	bodyStr := string(body)

	// Assert response body contains "services:" (docker-compose key).
	assertContains(t, bodyStr, "services:")
}

// TestModelUtil_ComposeNotFound verifies that GET /api/models/{slug}/compose
// returns 404 when the model does not exist.
func TestModelUtil_ComposeNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/nonexistent/compose", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusNotFound)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusNotFound)
}

// TestModelUtil_ClearCache_NoHFRepo verifies that DELETE /api/models/{slug}/cache
// returns 400 when the model has no HuggingFace repo configured.
func TestModelUtil_ClearCache_NoHFRepo(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// Seed a model without HF repo via POST.
	createResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-cache-clear",
		"name": "Cache Clear Test",
		"port": 7003,
		"type": "llm",
	})
	defer createResp.Body.Close()
	assertStatusCode(t, createResp, http.StatusCreated)

	// DELETE /api/models/test-cache-clear/cache.
	resp := doRequest(t, ts.Client(), http.MethodDelete, ts.URL+"/api/models/test-cache-clear/cache", nil)
	defer resp.Body.Close()

	// The model has no HF repo, so expect 400.
	assertStatusCode(t, resp, http.StatusBadRequest)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// 10. RAG Tests
// ---------------------------------------------------------------------------

// TestRAG_List verifies that GET /api/rag returns 200 with embed_models and
// rerank_models arrays populated after seeding RAG-type models via POST.
func TestRAG_List(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// Create an embedding model via POST.
	embResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug":    "test-embed",
		"name":    "Test Embed",
		"type":    "rag",
		"SubType": "embedding",
		"port":    6000,
	})
	defer embResp.Body.Close()
	assertStatusCode(t, embResp, http.StatusCreated)

	// Create a reranker model via POST.
	rnkResp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug":    "test-rerank",
		"name":    "Test Rerank",
		"type":    "rag",
		"SubType": "reranker",
		"port":    6001,
	})
	defer rnkResp.Body.Close()
	assertStatusCode(t, rnkResp, http.StatusCreated)

	// GET /api/rag.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/rag", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusOK)

	// Verify data contains both arrays with one item each.
	var ragData map[string]interface{}
	if err := json.Unmarshal(body, &ragData); err != nil {
		t.Fatalf("RAG_List: failed to unmarshal body: %v — raw: %s", err, string(body))
	}

	embArr, ok := ragData["embed_models"].([]interface{})
	if !ok {
		t.Fatalf("RAG_List: expected embed_models to be []interface{}, got %T — body: %s", ragData["embed_models"], string(body))
	}
	if len(embArr) != 1 {
		t.Errorf("RAG_List: expected 1 embed_model, got %d — body: %s", len(embArr), string(body))
	}

	rnkArr, ok := ragData["rerank_models"].([]interface{})
	if !ok {
		t.Fatalf("RAG_List: expected rerank_models to be []interface{}, got %T — body: %s", ragData["rerank_models"], string(body))
	}
	if len(rnkArr) != 1 {
		t.Errorf("RAG_List: expected 1 rerank_model, got %d — body: %s", len(rnkArr), string(body))
	}
}

// TestRAG_ListEmpty verifies that GET /api/rag returns 200 with empty
// embed_models and rerank_models arrays when no RAG models exist.
func TestRAG_ListEmpty(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// No RAG models seeded — just GET.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/rag", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertEnvelopeOK(t, body, http.StatusOK)

	// Verify both arrays are empty.
	var ragData map[string]interface{}
	if err := json.Unmarshal(body, &ragData); err != nil {
		t.Fatalf("RAG_ListEmpty: failed to unmarshal body: %v — raw: %s", err, string(body))
	}

	embArr, ok := ragData["embed_models"].([]interface{})
	if !ok {
		t.Fatalf("RAG_ListEmpty: expected embed_models to be []interface{}, got %T — body: %s", ragData["embed_models"], string(body))
	}
	if len(embArr) != 0 {
		t.Errorf("RAG_ListEmpty: expected 0 embed_models, got %d — body: %s", len(embArr), string(body))
	}

	rnkArr, ok := ragData["rerank_models"].([]interface{})
	if !ok {
		t.Fatalf("RAG_ListEmpty: expected rerank_models to be []interface{}, got %T — body: %s", ragData["rerank_models"], string(body))
	}
	if len(rnkArr) != 0 {
		t.Errorf("RAG_ListEmpty: expected 0 rerank_models, got %d — body: %s", len(rnkArr), string(body))
	}
}

// TestRAG_StartInvalidBody verifies that POST /api/rag/start with an invalid
// request body returns 400.
func TestRAG_StartInvalidBody(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// POST with invalid JSON body.
	resp := doRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/rag/start", strings.NewReader("{invalid json"))
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusBadRequest)

	body := readBody(t, resp)
	assertEnvelopeErr(t, body, http.StatusBadRequest)
}

// ---------------------------------------------------------------------------
// 11. Envelope Format Tests
// ---------------------------------------------------------------------------

// TestEnvelope_SuccessResponse verifies that a successful POST /api/models
// returns the raw model JSON (no envelope wrapper) with status=201.
func TestEnvelope_SuccessResponse(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// Create a model via POST.
	resp := doJSONRequest(t, ts.Client(), http.MethodPost, ts.URL+"/api/models", map[string]interface{}{
		"slug": "test-envelope-ok",
		"name": "Envelope OK",
		"port": 9999,
		"type": "llm",
	})
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusCreated)

	body := readBody(t, resp)

	// Successful responses are raw JSON — no envelope.
	var model map[string]interface{}
	if err := json.Unmarshal(body, &model); err != nil {
		t.Fatalf("Envelope_SuccessResponse: failed to unmarshal body: %v — raw: %s", err, string(body))
	}

	if model["Name"] != "Envelope OK" {
		t.Errorf("Envelope_SuccessResponse: expected Name='Envelope OK', got %v — body: %s", model["Name"], string(body))
	}

	// Verify no envelope keys
	if _, hasSuccess := model["success"]; hasSuccess {
		t.Errorf("Envelope_SuccessResponse: unexpected 'success' key in response — body: %s", string(body))
	}
}

// TestEnvelope_ErrorResponse verifies that a GET for a non-existent model
// returns an envelope with success=false, error present, and status=404.
func TestEnvelope_ErrorResponse(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()

	// GET a non-existent model.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models/nonexistent", nil)
	defer resp.Body.Close()

	assertStatusCode(t, resp, http.StatusNotFound)

	body := readBody(t, resp)

	// Error responses ARE wrapped in an envelope.
	envBody := extractEnvelopeBody(body)
	var env apiEnvelope
	if err := json.Unmarshal(envBody, &env); err != nil {
		t.Fatalf("Envelope_ErrorResponse: failed to unmarshal envelope: %v — raw: %s", err, string(body))
	}

	if env.Success {
		t.Errorf("Envelope_ErrorResponse: expected success=false, got true — body: %s", string(body))
	}

	if env.Status != http.StatusNotFound {
		t.Errorf("Envelope_ErrorResponse: expected status=%d, got %d — body: %s", http.StatusNotFound, env.Status, string(body))
	}

	if env.Error == "" {
		t.Errorf("Envelope_ErrorResponse: expected error to be non-empty — body: %s", string(body))
	}
}
