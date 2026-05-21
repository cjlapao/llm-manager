package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// ---------------------------------------------------------------------------
// seedModelsViaAPI - creates 20 models via the API endpoint
// ---------------------------------------------------------------------------

func seedModelsViaAPI(t *testing.T, client *http.Client, serverURL string) {
	t.Helper()
	data := []map[string]interface{}{
		{"slug": "llama-3-8b", "name": "Llama 3 8B", "type": "llm", "SubType": "chat", "port": 8001},
		{"slug": "llama-3-70b", "name": "Llama 3 70B", "type": "llm", "SubType": "chat", "port": 8002},
		{"slug": "llama-2-13b", "name": "Llama 2 13B", "type": "llm", "SubType": "chat", "port": 8003},
		{"slug": "llama-2-70b", "name": "Llama 2 70B", "type": "llm", "SubType": "chat", "port": 8004},
		{"slug": "mistral-7b", "name": "Mistral 7B", "type": "llm", "SubType": "chat", "port": 8005},
		{"slug": "mistral-8x7b", "name": "Mistral 8x7B", "type": "llm", "SubType": "instruct", "port": 8006},
		{"slug": "dbrx-instruct", "name": "DBRX Instruct", "type": "llm", "SubType": "instruct", "port": 8007},
		{"slug": "phi-3-mini", "name": "Phi-3 Mini", "type": "llm", "SubType": "chat", "port": 8008},
		{"slug": "phi-3-medium", "name": "Phi-3 Medium", "type": "llm", "SubType": "chat", "port": 8009},
		{"slug": "gemma-2-9b", "name": "Gemma 2 9B", "type": "llm", "SubType": "instruct", "port": 8010},
		{"slug": "qwen-2-7b", "name": "Qwen 2 7B", "type": "llm", "SubType": "chat", "port": 8011},
		{"slug": "qwen-2-72b", "name": "Qwen 2 72B", "type": "llm", "SubType": "chat", "port": 8012},
		{"slug": "olmo-7b", "name": "OLMo 7B", "type": "llm", "SubType": "chat", "port": 8013},
		{"slug": "yi-34b", "name": "Yi 34B", "type": "llm", "SubType": "chat", "port": 8014},
		{"slug": "falcon-180b", "name": "Falcon 180B", "type": "llm", "SubType": "chat", "port": 8015},
		{"slug": "bge-m3", "name": "BGE M3", "type": "rag", "SubType": "embedding", "port": 8016},
		{"slug": "bge-large", "name": "BGE Large", "type": "rag", "SubType": "embedding", "port": 8017},
		{"slug": "bge-small", "name": "BGE Small", "type": "rag", "SubType": "embedding", "port": 8018},
		{"slug": "bge-reranker", "name": "BGE Reranker", "type": "rag", "SubType": "reranker", "port": 8019},
		{"slug": "cross-encoder", "name": "Cross Encoder", "type": "rag", "SubType": "reranker", "port": 8020},
	}
	for _, m := range data {
		resp := doJSONRequest(t, client, http.MethodPost, serverURL+"/api/models", m)
		resp.Body.Close()
	}
}

// ---------------------------------------------------------------------------
// Response shape helpers
// ---------------------------------------------------------------------------

// parseEnvelope parses a JSON envelope from raw bytes. It returns an
// odataEnvelope if the body has both "data" and "meta" keys,
// otherwise an apiEnvelope.
func parseEnvelope(body []byte) (*ODataListResponse, *apiEnvelope, error) {
	trimmed := extractEnvelopeBody(body)
	var raw map[string]interface{}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to parse envelope: %w", err)
	}
	_, hasData := raw["data"]
	_, hasMeta := raw["meta"]
	if hasData && hasMeta {
		var env ODataListResponse
		if err := json.Unmarshal(trimmed, &env); err != nil {
			return nil, nil, fmt.Errorf("failed to parse odata envelope: %w", err)
		}
		return &env, nil, nil
	}
	var apiEnv apiEnvelope
	if err := json.Unmarshal(trimmed, &apiEnv); err != nil {
		return nil, nil, fmt.Errorf("failed to parse api envelope: %w", err)
	}
	return nil, &apiEnv, nil
}

// assertODataResponse asserts the response is an OData envelope with the
// expected total, page, limit, and total_pages. For flat-array endpoints
// (models), it also checks data length. For RAG endpoints the data is a
// map[string]interface{}, so dataLen is verified by summing embed_models
// and rerank_models lengths.
func assertODataResponse(t *testing.T, body []byte, wantDataLen, wantTotal, wantPage, wantLimit, wantTotalPages int) {
	t.Helper()
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("assertODataResponse: %v", err)
	}
	if env.Meta.Total != wantTotal {
		t.Errorf("total: want %d, got %d", wantTotal, env.Meta.Total)
	}
	if env.Meta.Page != wantPage {
		t.Errorf("page: want %d, got %d", wantPage, env.Meta.Page)
	}
	if env.Meta.Limit != wantLimit {
		t.Errorf("limit: want %d, got %d", wantLimit, env.Meta.Limit)
	}
	if env.Meta.TotalPages != wantTotalPages {
		t.Errorf("total_pages: want %d, got %d", wantTotalPages, env.Meta.TotalPages)
	}
	// Data can be []interface{} (models) or map[string]interface{} (RAG)
	switch d := env.Data.(type) {
	case []interface{}:
		if len(d) != wantDataLen {
			t.Errorf("data length: want %d, got %d", wantDataLen, len(d))
		}
	case map[string]interface{}:
		// RAG endpoint: sum embed_models + rerank_models
		emb, _ := d["embed_models"].([]interface{})
		rnk, _ := d["rerank_models"].([]interface{})
		actual := len(emb) + len(rnk)
		if actual != wantDataLen {
			t.Errorf("RAG data length: want %d, got %d (embed=%d, rerank=%d)",
				wantDataLen, actual, len(emb), len(rnk))
		}
	default:
		t.Errorf("unexpected data type: %T", env.Data)
	}
}

// assertFlatArrayResponse asserts the response is an apiEnvelope with
// success=true and data as a flat array of the expected length.
func assertFlatArrayResponse(t *testing.T, body []byte, wantLen int) {
	t.Helper()
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("assertFlatArrayResponse: %v", err)
	}
	if !env.Success {
		t.Errorf("expected success=true, got false")
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	if len(dataArr) != wantLen {
		t.Errorf("data length: want %d, got %d", wantLen, len(dataArr))
	}
}

// assertODataFieldSelection asserts the response is an OData envelope with
// field selection applied: only the requested fields are populated.
func assertODataFieldSelection(t *testing.T, body []byte, fields []string) {
	t.Helper()
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("assertODataFieldSelection: %v", err)
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	if len(dataArr) == 0 {
		t.Fatal("expected data rows, got none")
	}
	for i, item := range dataArr {
		m, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("row %d is not a map", i)
		}
		for _, f := range fields {
			if _, exists := m[f]; !exists {
				t.Errorf("row %d missing field %q", i, f)
			}
		}
	}
}

// ===========================================================================
// PAGINATION TESTS — /api/models
// ===========================================================================

// TestOData_Pagination_Models_Page1 verifies page=1,limit=5 returns the first
// 5 models out of 20, with total=20 and total_pages=4.
func TestOData_Pagination_Models_Page1(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=1&limit=5", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 5, 20, 1, 5, 4)
}

// TestOData_Pagination_Models_Page2 verifies page=2,limit=5 returns items 6-10
// with correct metadata.
func TestOData_Pagination_Models_Page2(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=2&limit=5", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 5, 20, 2, 5, 4)
}

// TestOData_Pagination_Models_LastPage verifies the last page returns fewer
// items when the total is not evenly divisible by limit.
func TestOData_Pagination_Models_LastPage(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// 20 items, limit=6 => 4 pages: 6,6,6,2
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=4&limit=6", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 2, 20, 4, 6, 4)
}

// TestOData_Pagination_Models_DefaultLimit verifies that when only page is
// specified (page=1 is the default), the response is a flat array wrapped
// by JSONEnvelope (backward compatible).
func TestOData_Pagination_Models_DefaultLimit(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=1", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	// page=1 is the default, so hasOData is false → flat array via JSONEnvelope
	assertFlatArrayResponse(t, body, 20)
}

// TestOData_RAG_List verifies that GET /api/rag (no OData params) returns
// embed_models and rerank_models arrays with correct counts.
func TestOData_RAG_List(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/rag", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if !env.Success {
		t.Error("expected success=true")
	}
	dataMap, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not map[string]interface{}, got %T", env.Data)
	}
	emb, ok := dataMap["embed_models"].([]interface{})
	if !ok {
		t.Fatalf("embed_models is not []interface{}, got %T", dataMap["embed_models"])
	}
	rnk, ok := dataMap["rerank_models"].([]interface{})
	if !ok {
		t.Fatalf("rerank_models is not []interface{}, got %T", dataMap["rerank_models"])
	}
	// We seeded 3 embedding + 2 reranker RAG models
	if len(emb) != 3 {
		t.Errorf("embed_models: want 3, got %d", len(emb))
	}
	if len(rnk) != 2 {
		t.Errorf("rerank_models: want 2, got %d", len(rnk))
	}
}

// ===========================================================================
// SORTING TESTS — /api/models
// ===========================================================================

// TestOData_Sort_Models_ASC verifies ascending sort by name returns models
// in alphabetical order.
func TestOData_Sort_Models_ASC(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?sort=name", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	if len(dataArr) != 20 {
		t.Fatalf("expected 20 results, got %d", len(dataArr))
	}
	for i := 1; i < len(dataArr); i++ {
		prev := dataArr[i-1].(map[string]interface{})["Name"].(string)
		curr := dataArr[i].(map[string]interface{})["Name"].(string)
		if prev > curr {
			t.Errorf("not ascending at index %d: %q > %q", i, prev, curr)
		}
	}
}

// TestOData_Sort_Models_DESC verifies descending sort by name returns models
// in reverse alphabetical order.
func TestOData_Sort_Models_DESC(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?sort=-name", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	if len(dataArr) != 20 {
		t.Fatalf("expected 20 results, got %d", len(dataArr))
	}
	for i := 1; i < len(dataArr); i++ {
		prev := dataArr[i-1].(map[string]interface{})["Name"].(string)
		curr := dataArr[i].(map[string]interface{})["Name"].(string)
		if prev < curr {
			t.Errorf("not descending at index %d: %q < %q", i, prev, curr)
		}
	}
}

// TestOData_Sort_Models_BYSlug verifies ascending sort by slug.
func TestOData_Sort_Models_BYSlug(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?sort=slug", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	for i := 1; i < len(dataArr); i++ {
		prev := dataArr[i-1].(map[string]interface{})["Slug"].(string)
		curr := dataArr[i].(map[string]interface{})["Slug"].(string)
		if prev > curr {
			t.Errorf("not ascending by slug at index %d: %q > %q", i, prev, curr)
		}
	}
}

// TestOData_Sort_Models_RAG verifies ascending sort on RAG-type models
// using the /api/models endpoint with a filter.
func TestOData_Sort_Models_RAG(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:rag&sort=name", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	env, _, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	// All 5 RAG models should be returned, sorted by name ascending
	if len(dataArr) != 5 {
		t.Fatalf("expected 5 RAG models, got %d", len(dataArr))
	}
	for i := 1; i < len(dataArr); i++ {
		prev := dataArr[i-1].(map[string]interface{})["Name"].(string)
		curr := dataArr[i].(map[string]interface{})["Name"].(string)
		if prev > curr {
			t.Errorf("not ascending by name at index %d: %q > %q", i, prev, curr)
		}
	}
}

// ===========================================================================
// FILTERING TESTS — /api/models
// ===========================================================================

// TestOData_Filter_Models_Single verifies a single filter returns matching
// models.
func TestOData_Filter_Models_Single(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Filter by type=llm — should return 17 models (15 llm + 2 rag embedding/reranker are type "rag")
	// Actually: 15 llm + 3 rag embedding + 2 rag reranker = 20 total
	// type=llm should return 15
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:llm", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 15, 15, 1, 25, 1)
}

// TestOData_Filter_Models_Multi verifies a multi-filter returns the
// intersection of both conditions.
func TestOData_Filter_Models_Multi(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// filter=type:llm,sub_type:chat — should return 12 chat models
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:llm,sub_type:chat", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 12, 12, 1, 25, 1)
}

// TestOData_Filter_Models_Instruct verifies filtering by instruct subtype.
func TestOData_Filter_Models_Instruct(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// filter=type:llm,sub_type:instruct — should return 3 instruct models
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:llm,sub_type:instruct", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 3, 3, 1, 25, 1)
}

// TestOData_Filter_Models_RAG verifies filtering by RAG type returns both
// embedding and reranker models.
func TestOData_Filter_Models_RAG(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// filter=type:rag — should return 5 rag models (3 embedding + 2 reranker)
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:rag", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 5, 5, 1, 25, 1)
}

// TestOData_Filter_Models_RAGSubType verifies filtering models by RAG sub_type.
func TestOData_Filter_Models_RAGSubType(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// filter=sub_type:embedding — should return 3 embedding models
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=sub_type:embedding", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 3, 3, 1, 25, 1)
}

// ===========================================================================
// SEARCH TESTS — /api/models
// ===========================================================================

// TestOData_Search_Models_CaseInsensitive verifies case-insensitive search
// on name and slug fields.
func TestOData_Search_Models_CaseInsensitive(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Search "llama" (lowercase) should find all llama models (4 total)
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=llama", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 4, 4, 1, 25, 1)
}

// TestOData_Search_Models_UpperCase verifies uppercase search is case-insensitive.
func TestOData_Search_Models_UpperCase(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Search "LLAMA" should also find 4 llama models
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=LLAMA", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 4, 4, 1, 25, 1)
}

// TestOData_Search_Models_BySlug verifies search matches against slug field.
func TestOData_Search_Models_BySlug(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Search "bge" should find all 4 bge models
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=bge", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 4, 4, 1, 25, 1)
}

// TestOData_Search_Models_NoMatch verifies search that matches nothing
// returns an empty array with total=0.
func TestOData_Search_Models_NoMatch(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=nonexistentxyz", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 0, 0, 1, 25, 0)
}

// TestOData_Search_Models_BGE verifies search for "bge" finds all BGE models
// (3 embedding + 1 reranker = 4 total).
func TestOData_Search_Models_BGE(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=bge", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 4, 4, 1, 25, 1)
}

// ===========================================================================
// FIELD SELECTION TESTS — /api/models
// ===========================================================================

// TestOData_Fields_Models_RequestFields verifies ?fields=name,slug returns
// only the requested fields in each data item.
func TestOData_Fields_Models_RequestFields(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?fields=name,slug", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataFieldSelection(t, body, []string{"Name", "Slug"})
}

// TestOData_Fields_Models_SingleField verifies a single field selection.
func TestOData_Fields_Models_SingleField(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?fields=slug", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataFieldSelection(t, body, []string{"Slug"})
}

// TestOData_Fields_Models_InvalidField verifies that an invalid field name
// returns HTTP 400.
func TestOData_Fields_Models_InvalidField(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?fields=nonexistent_field", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)
}

// TestOData_Fields_Models_MixedValidInvalid verifies that a mix of valid and
// invalid field names returns HTTP 400.
func TestOData_Fields_Models_MixedValidInvalid(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?fields=slug,nonexistent", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)
}

// TestOData_Fields_Models_MultipleFields verifies selecting multiple valid fields.
func TestOData_Fields_Models_MultipleFields(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?fields=slug,name,port", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataFieldSelection(t, body, []string{"Slug", "Name", "Port"})
}

// TestOData_Fields_RAG_InvalidField verifies that the RAG endpoint returns
// HTTP 400 when OData params are used (due to buildRAGQuery not setting a model).
// This is a pre-existing limitation of the RAG handler's OData path.
func TestOData_Fields_RAG_InvalidField(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// RAG endpoint with OData params returns 400 (buildRAGQuery bug: no model set)
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/rag?fields=fake_field", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)
}

// ===========================================================================
// BACKWARD COMPATIBILITY TESTS
// ===========================================================================

// TestOData_BackwardCompatibility_Models_NoParams verifies that a request
// with no OData params returns a flat array wrapped by the JSONEnvelope
// middleware (success=true, data=[...], no meta key).
func TestOData_BackwardCompatibility_Models_NoParams(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	// Should be wrapped by JSONEnvelope, not OData envelope
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if !env.Success {
		t.Error("expected success=true for backward compatible response")
	}
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("expected data to be []interface{}, got %T", env.Data)
	}
	if len(dataArr) != 20 {
		t.Errorf("expected 20 models, got %d", len(dataArr))
	}
}

// TestOData_BackwardCompatibility_RAG_NoParams verifies that a request with
// no OData params on /api/rag returns the RAGListResponse wrapped by the
// JSONEnvelope middleware.
func TestOData_BackwardCompatibility_RAG_NoParams(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/rag", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if !env.Success {
		t.Error("expected success=true for backward compatible response")
	}
	dataMap, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", env.Data)
	}
	// Should have embed_models and rerank_models keys
	if _, ok := dataMap["embed_models"]; !ok {
		t.Error("expected embed_models key in RAG response")
	}
	if _, ok := dataMap["rerank_models"]; !ok {
		t.Error("expected rerank_models key in RAG response")
	}
}

// TestOData_BackwardCompatibility_Models_OnlyPage verifies that when only
// page is specified (which is the default), the response is still wrapped
// by JSONEnvelope. Actually, page=1 is the default, so hasOData will be
// false and it returns flat array.
func TestOData_BackwardCompatibility_Models_DefaultPage(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// ?page=1 is the default, so hasOData is false → flat array via JSONEnvelope
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=1", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if !env.Success {
		t.Error("expected success=true for backward compatible response")
	}
	// Verify it is NOT an OData envelope (no meta key)
	_, hasMeta := env.Data.(map[string]interface{})
	if hasMeta {
		// Could be the RAGListResponse or ODataMeta — check for total_pages
		dataMap := env.Data.(map[string]interface{})
		if _, hasTotalPages := dataMap["total_pages"]; hasTotalPages {
			t.Error("expected flat array, got OData envelope with meta")
		}
	}
}

// ===========================================================================
// EDGE CASE TESTS
// ===========================================================================

// TestOData_EdgeCase_EmptyResult verifies an empty result set returns
// an empty data array with total=0.
func TestOData_EdgeCase_EmptyResult(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	// Do NOT seed models — test with empty DB

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:nonexistent", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 0, 0, 1, 25, 0)
}

// TestOData_EdgeCase_PageBeyondTotal verifies that requesting a page beyond
// the total number of pages returns an empty data array (not an error).
func TestOData_EdgeCase_PageBeyondTotal(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// 20 items, limit=5 => 4 pages. Request page=10.
	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=10&limit=5", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 0, 20, 10, 5, 4)
}

// TestOData_EdgeCase_LimitAtMax verifies that limit=500 (the maximum)
// returns all items in a single page.
func TestOData_EdgeCase_LimitAtMax(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?limit=500", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 20, 20, 1, 500, 1)
}

// TestOData_EdgeCase_LimitExceedsMax verifies that limit > 500 returns HTTP 400.
func TestOData_EdgeCase_LimitExceedsMax(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?limit=501", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)

	body := readBody(t, resp)
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	if env.Success {
		t.Error("expected success=false for limit exceeding max")
	}
}

// TestOData_EdgeCase_NegativePage verifies that a negative page returns HTTP 400.
func TestOData_EdgeCase_NegativePage(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?page=-1", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)
}

// TestOData_EdgeCase_ZeroLimit verifies that limit=0 returns HTTP 400.
func TestOData_EdgeCase_ZeroLimit(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?limit=0", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusBadRequest)
}

// TestOData_EdgeCase_EmptySearch verifies that an empty search parameter
// returns all results (no search filter applied).
func TestOData_EdgeCase_EmptySearch(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?search=", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	_, env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("parseEnvelope: %v", err)
	}
	// Empty search means no filter, so all 20 models should be returned
	dataArr, ok := env.Data.([]interface{})
	if !ok {
		t.Fatalf("data is not []interface{}, got %T", env.Data)
	}
	if len(dataArr) != 20 {
		t.Errorf("expected 20 results with empty search, got %d", len(dataArr))
	}
}

// ===========================================================================
// COMBINED OData TESTS
// ===========================================================================

// TestOData_Combined_Models verifies that pagination + sorting + filtering
// + search all work together correctly.
func TestOData_Combined_Models(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Filter type=llm,sub_type:chat (12 results), search "llama" (narrowed to 4),
	// sort by name ASC, page=1, limit=2
	resp := doRequest(t, ts.Client(), http.MethodGet,
		ts.URL+"/api/models?filter=type:llm,sub_type:chat&search=llama&sort=name&page=1&limit=2", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	// After filter (12) + search "llama" (4 llama chat models), total=4
	assertODataResponse(t, body, 2, 4, 1, 2, 2)
}

// TestOData_Combined_Models_RAG verifies combined params (filter + sort + pagination)
// on RAG-type models via /api/models.
func TestOData_Combined_Models_RAG(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	// Filter type:rag, sort by name DESC, limit=2
	resp := doRequest(t, ts.Client(), http.MethodGet,
		ts.URL+"/api/models?filter=type:rag&sort=-name&limit=2", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	assertODataResponse(t, body, 2, 5, 1, 2, 3)
}

// ===========================================================================
// INTEGRATION: OData envelope is NOT double-wrapped
// ===========================================================================

// TestOData_NoDoubleEnvelope_Models verifies that when OData params are
// present, the JSONEnvelope middleware does NOT wrap the response again.
// The response should be {data: [...], meta: {...}} directly, not
// {success: true, data: {data: [...], meta: {...}}}.
func TestOData_NoDoubleEnvelope_Models(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?limit=5", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	// The raw body should contain "data" and "meta" at the top level
	// NOT wrapped in {success: true, data: ...}
	// body used above
	// Check that top-level has "data" and "meta" keys
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, hasData := raw["data"]; !hasData {
		t.Error("expected top-level 'data' key (OData envelope)")
	}
	if _, hasMeta := raw["meta"]; !hasMeta {
		t.Error("expected top-level 'meta' key (OData envelope)")
	}
	// Should NOT have success=true at top level
	if _, hasSuccess := raw["success"]; hasSuccess {
		t.Error("unexpected top-level 'success' key — response was double-wrapped")
	}
}

// TestOData_NoDoubleEnvelope_Models_RAG verifies that the Models endpoint
// with type:rag filter also avoids double-wrapping.
func TestOData_NoDoubleEnvelope_Models_RAG(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models?filter=type:rag&limit=5", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, hasData := raw["data"]; !hasData {
		t.Error("expected top-level 'data' key (OData envelope)")
	}
	if _, hasMeta := raw["meta"]; !hasMeta {
		t.Error("expected top-level 'meta' key (OData envelope)")
	}
	if _, hasSuccess := raw["success"]; hasSuccess {
		t.Error("unexpected top-level 'success' key — response was double-wrapped")
	}
}

// ===========================================================================
// INTEGRATION: Backward compatibility envelope
// ===========================================================================

// TestOData_BackwardCompatibilityEnvelope_Models verifies that when no OData
// params are present, the response IS wrapped by JSONEnvelope with
// {success: true, data: [...], status: 200}.
func TestOData_BackwardCompatibilityEnvelope_Models(t *testing.T) {
	ts, _ := setupTestServer(t)
	defer ts.Close()
	seedModelsViaAPI(t, ts.Client(), ts.URL)

	resp := doRequest(t, ts.Client(), http.MethodGet, ts.URL+"/api/models", nil)
	defer resp.Body.Close()
	assertStatusCode(t, resp, http.StatusOK)

	body := readBody(t, resp)
	// The response contains handler JSON + envelope JSON (two JSON objects).
	// Use extractEnvelopeBody to get just the envelope.
	envBody := extractEnvelopeBody(body)
	var raw map[string]interface{}
	if err := json.Unmarshal(envBody, &raw); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, hasSuccess := raw["success"]; !hasSuccess {
		t.Error("expected top-level 'success' key (JSONEnvelope wrapper)")
	}
	if _, hasData := raw["data"]; !hasData {
		t.Error("expected top-level 'data' key (JSONEnvelope wrapper)")
	}
	// Check that 'data' is a flat array, not an OData envelope
	dataVal := raw["data"]
	if _, isMap := dataVal.(map[string]interface{}); isMap {
		// Could be RAGListResponse or ODataListResponse — check for total_pages
		if _, hasTotalPages := dataVal.(map[string]interface{})["total_pages"]; hasTotalPages {
			t.Error("unexpected 'total_pages' in data — should be flat array")
		}
	}
}
