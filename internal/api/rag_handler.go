package api

import (
	"encoding/json"
	"net/http"
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

// ListRAG lists all RAG models (embedding and reranker) with their container status.
//
// GET /api/rag
func (h *RAGHandler) ListRAG(w http.ResponseWriter, r *http.Request) {
	// Fetch embedding models
	embedModels, err := h.DB.ListModelsByTypeSubType("rag", "embedding")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list embedding models: "+err.Error())
		return
	}

	// Fetch reranker models
	rerankModels, err := h.DB.ListModelsByTypeSubType("rag", "reranker")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list reranker models: "+err.Error())
		return
	}

	// Build response with container status for each model
	resp := RAGListResponse{
		EmbedModels:  make([]RAGModelInfo, 0, len(embedModels)),
		RerankModels: make([]RAGModelInfo, 0, len(rerankModels)),
	}

	for _, m := range embedModels {
		status, err := h.ContainerService.GetModelStatus(m.Slug)
		if err != nil {
			// If we can't get status, mark as unknown
			resp.EmbedModels = append(resp.EmbedModels, RAGModelInfo{
				Slug:      m.Slug,
				Name:      m.Name,
				Container: m.Container,
				Port:      m.Port,
				Status:    "unknown",
			})
		} else {
			resp.EmbedModels = append(resp.EmbedModels, RAGModelInfo{
				Slug:      status.Slug,
				Name:      status.Name,
				Container: status.Container,
				Port:      status.Port,
				Status:    status.Status,
			})
		}
	}

	for _, m := range rerankModels {
		status, err := h.ContainerService.GetModelStatus(m.Slug)
		if err != nil {
			resp.RerankModels = append(resp.RerankModels, RAGModelInfo{
				Slug:      m.Slug,
				Name:      m.Name,
				Container: m.Container,
				Port:      m.Port,
				Status:    "unknown",
			})
		} else {
			resp.RerankModels = append(resp.RerankModels, RAGModelInfo{
				Slug:      status.Slug,
				Name:      status.Name,
				Container: status.Container,
				Port:      status.Port,
				Status:    status.Status,
			})
		}
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
