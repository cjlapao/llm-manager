package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
	"gorm.io/gorm"
)

// RAGHandler handles RAG-related API endpoints for listing, starting, and stopping
// embedding and reranker model containers.
type RAGHandler struct {
	*APIContext
}

// RAGModelInfo represents a RAG model with its container status.
type RAGModelInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Container string `json:"container"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

// RAGListResponse is the response shape for GET /api/rag.
type RAGListResponse struct {
	EmbedModels  []RAGModelInfo `json:"embed_models"`
	RerankModels []RAGModelInfo `json:"rerank_models"`
}

// ODataListResponse is the wrapped response shape for OData-enabled list endpoints.
type ODataListResponse struct {
	Data interface{} `json:"data"`
	Meta ODataMeta   `json:"meta"`
}

// ODataMeta contains pagination metadata.
type ODataMeta struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalPages int `json:"total_pages"`
}

// StartRAGRequest is the request body for POST /api/rag/start.
type StartRAGRequest struct {
	EmbedSlug  string `json:"embed_slug"`
	RerankSlug string `json:"rerank_slug"`
}

// StopRAGRequest is the request body for POST /api/rag/stop.
type StopRAGRequest struct {
	EmbedSlug  string `json:"embed_slug"`
	RerankSlug string `json:"rerank_slug"`
}

// StartRAGResponse is the response body for POST /api/rag/start.
type StartRAGResponse struct {
	Started []string `json:"started"`
}

// StopRAGResponse is the response body for POST /api/rag/stop.
type StopRAGResponse struct {
	Stopped []string `json:"stopped"`
}

// ragModelWhiteList defines the allowed fields for RAG model filtering/sorting.
// RAG models are stored as Model records (type="rag", sub_type="embedding"|"reranker"),
// so we use the "models" table white-list plus the computed "status" field.
var ragModelWhiteList = []string{
	"slug", "name", "type", "sub_type", "container", "port",
	"engine_type", "created_at", "updated_at", "status",
}

func initRagWhiteListSet() map[string]struct{} {
	set := make(map[string]struct{}, len(ragModelWhiteList))
	for _, f := range ragModelWhiteList {
		set[f] = struct{}{}
	}
	return set
}

var ragWhiteListSet = initRagWhiteListSet()

// validateRagFields checks that every requested field is in the RAG white-list.
func validateRagFields(fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	unknown := make([]string, 0)
	for _, f := range fields {
		if _, ok := ragWhiteListSet[f]; !ok {
			unknown = append(unknown, f)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unsupported field(s) for RAG: %s (allowed: %s)",
			strings.Join(unknown, ", "),
			strings.Join(ragModelWhiteList, ", "))
	}
	return nil
}

// modelsToRAGInfo converts a slice of models.Model to []RAGModelInfo with container status.
func modelsToRAGInfo(h *RAGHandler, modelList []models.Model) []RAGModelInfo {
	result := make([]RAGModelInfo, 0, len(modelList))
	for _, m := range modelList {
		status, err := h.ContainerService.GetModelStatus(m.Slug)
		if err != nil {
			result = append(result, RAGModelInfo{
				Slug:      m.Slug,
				Name:      m.Name,
				Container: m.Container,
				Port:      m.Port,
				Status:    "unknown",
			})
		} else {
			result = append(result, RAGModelInfo{
				Slug:      status.Slug,
				Name:      status.Name,
				Container: status.Container,
				Port:      status.Port,
				Status:    status.Status,
			})
		}
	}
	return result
}

// buildRAGQuery builds a GORM query for RAG models of the given subtype,
// applying sort, filter, and search. It returns the query and total count.
func buildRAGQuery(db *gorm.DB, subType string, opts ODataQuery) (*gorm.DB, int64, error) {
	q := db.Where("type = ? AND sub_type = ?", "rag", subType)

	// --- Sort ---
	if opts.SortField != "" {
		if _, ok := ragWhiteListSet[opts.SortField]; !ok {
			return nil, 0, fmt.Errorf("sort field %q not allowed for RAG", opts.SortField)
		}
		orderCol := opts.SortField
		if opts.SortDir == SortDESC {
			orderCol = opts.SortField + " DESC"
		}
		q = q.Order(orderCol)
	}

	// --- Filter ---
	if len(opts.Filter) > 0 {
		for col, val := range opts.Filter {
			if _, ok := ragWhiteListSet[col]; !ok {
				return nil, 0, fmt.Errorf("filter field %q not allowed for RAG", col)
			}
			q = q.Where(col+" = ?", val)
		}
	}

	// --- Search ---
	if opts.Search != "" {
		searchTerm := "%" + opts.Search + "%"
		q = q.Where("name LIKE ? OR slug LIKE ?", searchTerm, searchTerm)
	}

	// --- Count ---
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count RAG models: %w", err)
	}

	// --- Pagination ---
	if opts.Limit > 0 {
		offset := (opts.Page - 1) * opts.Limit
		q = q.Limit(opts.Limit).Offset(offset)
	}

	return q, total, nil
}

// ListRAG lists all RAG models (embedding and reranker) with their container status.
//
// GET /api/rag
func (h *RAGHandler) ListRAG(w http.ResponseWriter, r *http.Request) {
	// Parse OData query parameters
	opts, err := ParseODataQuery(r)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid query parameters: "+err.Error())
		return
	}

	// Check if any OData params are present
	hasOData := opts.Page != 1 || opts.Limit != 25 || opts.SortField != "" ||
		len(opts.Filter) > 0 || opts.Search != "" || len(opts.Fields) > 0

	if !hasOData {
		// No OData params — return flat array (backward compatible)
		embedModels, err := h.DB.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list embedding models: "+err.Error())
			return
		}

		rerankModels, err := h.DB.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list reranker models: "+err.Error())
			return
		}

		resp := RAGListResponse{
			EmbedModels:  modelsToRAGInfo(h, embedModels),
			RerankModels: modelsToRAGInfo(h, rerankModels),
		}

		WriteJSON(w, http.StatusOK, resp)
		return
	}

	// OData params present — apply filters and return wrapped response
	var embedModels, rerankModels []models.Model
	var embedTotal, rerankTotal int64

	// --- Embedding models ---
	if opts.SortField != "" || len(opts.Filter) > 0 || opts.Search != "" || opts.Limit > 0 {
		q, total, err := buildRAGQuery(h.DB.DB(), "embedding", opts)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		embedTotal = total
		if err := q.Find(&embedModels).Error; err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query embedding models: "+err.Error())
			return
		}
	} else {
		embedModels, err = h.DB.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list embedding models: "+err.Error())
			return
		}
		embedTotal = int64(len(embedModels))
	}

	// --- Reranker models ---
	if opts.SortField != "" || len(opts.Filter) > 0 || opts.Search != "" || opts.Limit > 0 {
		q, total, err := buildRAGQuery(h.DB.DB(), "reranker", opts)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		rerankTotal = total
		if err := q.Find(&rerankModels).Error; err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query reranker models: "+err.Error())
			return
		}
	} else {
		rerankModels, err = h.DB.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list reranker models: "+err.Error())
			return
		}
		rerankTotal = int64(len(rerankModels))
	}

	// Convert to RAGModelInfo with status
	embedInfo := modelsToRAGInfo(h, embedModels)
	rerankInfo := modelsToRAGInfo(h, rerankModels)

	// Calculate total pages
	totalRecords := embedTotal + rerankTotal
	limit := opts.Limit
	if limit == 0 {
		limit = 25
	}
	totalPages := int((totalRecords + int64(limit) - 1) / int64(limit))

	resp := ODataListResponse{
		Data: RAGListResponse{
			EmbedModels:  embedInfo,
			RerankModels: rerankInfo,
		},
		Meta: ODataMeta{
			Total:      int(totalRecords),
			Page:       opts.Page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}

	WriteJSON(w, http.StatusOK, resp)
}

// StartRAG starts RAG model containers based on the request slugs.
//
// POST /api/rag/start
func (h *RAGHandler) StartRAG(w http.ResponseWriter, r *http.Request) {
	var req StartRAGRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Resolve embed slug: if empty, pick the first available embedding model
	embedSlug := req.EmbedSlug
	if embedSlug == "" {
		ms, err := h.DB.ListModelsByTypeSubType("rag", "embedding")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list embedding models: "+err.Error())
			return
		}
		if len(ms) == 0 {
			WriteError(w, http.StatusNotFound, "no embedding models available")
			return
		}
		embedSlug = ms[0].Slug
	}

	// Resolve rerank slug: if empty, pick the first available reranker model
	rerankSlug := req.RerankSlug
	if rerankSlug == "" {
		ms, err := h.DB.ListModelsByTypeSubType("rag", "reranker")
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list reranker models: "+err.Error())
			return
		}
		if len(ms) == 0 {
			WriteError(w, http.StatusNotFound, "no reranker models available")
			return
		}
		rerankSlug = ms[0].Slug
	}

	// Validate both resolved slugs exist in DB before starting
	if _, err := h.DB.GetModel(embedSlug); err != nil {
		WriteError(w, http.StatusNotFound, "embedding model not found: "+embedSlug)
		return
	}
	if _, err := h.DB.GetModel(rerankSlug); err != nil {
		WriteError(w, http.StatusNotFound, "reranker model not found: "+rerankSlug)
		return
	}

	started := make([]string, 0, 2)

	// Always start both resolved slugs (matching CLI behavior)
	if err := h.ContainerService.StartModelBySlugWithAllow(embedSlug, false); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to start embedding model: "+err.Error())
		return
	}
	started = append(started, embedSlug)

	if err := h.ContainerService.StartModelBySlugWithAllow(rerankSlug, false); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to start reranker model: "+err.Error())
		return
	}
	started = append(started, rerankSlug)

	WriteJSON(w, http.StatusOK, StartRAGResponse{Started: started})
}

// StopRAG stops RAG model containers based on the request slugs.
//
// POST /api/rag/stop
func (h *RAGHandler) StopRAG(w http.ResponseWriter, r *http.Request) {
	var req StopRAGRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	stopped := make([]string, 0, 2)

	// If both slugs are omitted, stop all running containers of both subtypes
	if req.EmbedSlug == "" && req.RerankSlug == "" {
		if err := h.ContainerService.StopAllBySubType("rag", "embedding"); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to stop embedding containers: "+err.Error())
			return
		}
		if err := h.ContainerService.StopAllBySubType("rag", "reranker"); err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to stop reranker containers: "+err.Error())
			return
		}
		// List all models to report what was stopped
		embedModels, _ := h.DB.ListModelsByTypeSubType("rag", "embedding")
		for _, m := range embedModels {
			stopped = append(stopped, m.Slug)
		}
		rerankModels, _ := h.DB.ListModelsByTypeSubType("rag", "reranker")
		for _, m := range rerankModels {
			stopped = append(stopped, m.Slug)
		}
	} else {
		// Stop specific embed model if provided
		if req.EmbedSlug != "" {
			if _, err := h.DB.GetModel(req.EmbedSlug); err != nil {
				WriteError(w, http.StatusNotFound, "embedding model not found: "+req.EmbedSlug)
				return
			}
			if err := h.ContainerService.StopModelBySlug(req.EmbedSlug); err != nil {
				WriteError(w, http.StatusInternalServerError, "failed to stop embedding model: "+err.Error())
				return
			}
			stopped = append(stopped, req.EmbedSlug)
		}

		// Stop specific rerank model if provided
		if req.RerankSlug != "" {
			if _, err := h.DB.GetModel(req.RerankSlug); err != nil {
				WriteError(w, http.StatusNotFound, "reranker model not found: "+req.RerankSlug)
				return
			}
			if err := h.ContainerService.StopModelBySlug(req.RerankSlug); err != nil {
				WriteError(w, http.StatusInternalServerError, "failed to stop reranker model: "+err.Error())
				return
			}
			stopped = append(stopped, req.RerankSlug)
		}
	}

	WriteJSON(w, http.StatusOK, StopRAGResponse{Stopped: stopped})
}
