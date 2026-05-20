package api

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/user/llm-manager/internal/database/models"
)

// ModelHandler provides HTTP handlers for LLM model CRUD operations.
type ModelHandler struct {
	*APIContext
}

// ListModels handles GET /api/models — returns all models as a JSON array,
// or a wrapped OData response when query parameters are present.
func (h *ModelHandler) ListModels(w http.ResponseWriter, r *http.Request) {
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
		allModels, err := h.ModelService.ListModels()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to list models: "+err.Error())
			return
		}
		WriteJSON(w, http.StatusOK, allModels)
		return
	}

	// OData params present — apply filters and return wrapped response
	db := h.DB.DB()

	// Apply OData filters
	filtered, total, err := ApplyODataFilters(db.Model(&models.Model{}), ODataOptions{
		ODataQuery: opts,
		ModelTable: "models",
		ModelType:  &models.Model{},
	})
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var allModels []models.Model
	if err := filtered.Find(&allModels).Error; err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to query models: "+err.Error())
		return
	}

	// Calculate total pages
	limit := opts.Limit
	if limit == 0 {
		limit = 25
	}
	totalPages := int((total + int64(limit) - 1) / int64(limit))

	resp := ODataListResponse{
		Data: allModels,
		Meta: ODataMeta{
			Total:      int(total),
			Page:       opts.Page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	}

	WriteJSON(w, http.StatusOK, resp)
}

// CreateModel handles POST /api/models — creates a new model from a JSON body.
func (h *ModelHandler) CreateModel(w http.ResponseWriter, r *http.Request) {
	var model models.Model
	if err := json.NewDecoder(r.Body).Decode(&model); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return
	}

	if err := h.ModelService.CreateModel(&model); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create model: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, model)
}

// GetModel handles GET /api/models/{slug} — returns a single model by slug.
func (h *ModelHandler) GetModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	model, err := h.ModelService.GetModel(slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, "model not found: "+slug)
		return
	}

	WriteJSON(w, http.StatusOK, model)
}

// UpdateModel handles PUT /api/models/{slug} — partial update via JSON map.
func (h *ModelHandler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		WriteError(w, http.StatusBadRequest, "slug is required")
		return
	}

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return
	}

	if err := h.ModelService.UpdateModel(slug, updates); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update model: "+err.Error())
		return
	}

	updated, err := h.ModelService.GetModel(slug)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to retrieve updated model: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, updated)
}

// DeleteModel handles DELETE /api/models/{slug} — deletes a model by slug.
func (h *ModelHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		WriteError(w, http.StatusBadRequest, "slug is required")
		return
	}

	if err := h.ModelService.DeleteModel(slug); err != nil {
		WriteError(w, http.StatusNotFound, "model not found: "+slug)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ModelInfoResponse is the enriched response shape for GET /api/models/{slug}/info.
// It includes the model record with JSON string fields parsed into typed objects.
type ModelInfoResponse struct {
	Slug                 string                 `json:"slug"`
	Type                 string                 `json:"type"`
	SubType              string                 `json:"sub_type"`
	Name                 string                 `json:"name"`
	HFRepo               string                 `json:"hf_repo"`
	Container            string                 `json:"container"`
	Port                 int                    `json:"port"`
	EngineType           string                 `json:"engine_type"`
	InputTokenCost       float64                `json:"input_token_cost"`
	OutputTokenCost      float64                `json:"output_token_cost"`
	Capabilities         []string               `json:"capabilities"`
	LiteLLMParams        map[string]interface{} `json:"litellm_params"`
	ModelInfo            map[string]interface{} `json:"model_info"`
	Default              bool                   `json:"default"`
	TotalParamsB         *float64               `json:"total_params_b"`
	ActiveParamsB        *float64               `json:"active_params_b"`
	IsMoe                *bool                  `json:"is_moe"`
	AttentionLayers      *int                   `json:"attention_layers"`
	GdnLayers            *int                   `json:"gdn_layers"`
	NumKvHeads           *int                   `json:"num_kv_heads"`
	HeadDim              *int                   `json:"head_dim"`
	SupportsMtp          *bool                  `json:"supports_mtp"`
	DefaultContext       *int                   `json:"default_context"`
	MaxContext           *int                   `json:"max_context"`
	QuantBytesPerParam   *float64               `json:"quant_bytes_per_param"`
	MaxNumSeqs           *int                   `json:"max_num_seqs"`
	MaxNumBatchedTokens  *int                   `json:"max_num_batched_tokens"`
	SpeculativeDecoding  *string                `json:"speculative_decoding"`
	NumSpeculativeTokens *int                   `json:"num_speculative_tokens"`
	CreatedAt            string                 `json:"created_at"`
	UpdatedAt            string                 `json:"updated_at"`
}

// GetModelInfo handles GET /api/models/{slug}/info — returns model with parsed
// JSON fields (LiteLLMParams, ModelInfo, Capabilities) as objects.
func (h *ModelHandler) GetModelInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	model, err := h.ModelService.GetModel(slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, "model not found: "+slug)
		return
	}

	// Parse capabilities JSON string
	var capabilities []string
	if model.Capabilities != "" {
		if err := json.Unmarshal([]byte(model.Capabilities), &capabilities); err != nil {
			capabilities = []string{}
		}
	}

	// Parse litellm_params JSON string
	var liteLLMParams map[string]interface{}
	if model.LiteLLMParams != "" {
		if err := json.Unmarshal([]byte(model.LiteLLMParams), &liteLLMParams); err != nil {
			liteLLMParams = map[string]interface{}{}
		}
	}

	// Parse model_info JSON string
	var modelInfo map[string]interface{}
	if model.ModelInfo != "" {
		if err := json.Unmarshal([]byte(model.ModelInfo), &modelInfo); err != nil {
			modelInfo = map[string]interface{}{}
		}
	}

	response := ModelInfoResponse{
		Slug:                 model.Slug,
		Type:                 model.Type,
		SubType:              model.SubType,
		Name:                 model.Name,
		HFRepo:               model.HFRepo,
		Container:            model.Container,
		Port:                 model.Port,
		EngineType:           model.EngineType,
		InputTokenCost:       model.InputTokenCost,
		OutputTokenCost:      model.OutputTokenCost,
		Capabilities:         capabilities,
		LiteLLMParams:        liteLLMParams,
		ModelInfo:            modelInfo,
		Default:              model.Default,
		TotalParamsB:         model.TotalParamsB,
		ActiveParamsB:        model.ActiveParamsB,
		IsMoe:                model.IsMoe,
		AttentionLayers:      model.AttentionLayers,
		GdnLayers:            model.GdnLayers,
		NumKvHeads:           model.NumKvHeads,
		HeadDim:              model.HeadDim,
		SupportsMtp:          model.SupportsMtp,
		DefaultContext:       model.DefaultContext,
		MaxContext:           model.MaxContext,
		QuantBytesPerParam:   model.QuantBytesPerParam,
		MaxNumSeqs:           model.MaxNumSeqs,
		MaxNumBatchedTokens:  model.MaxNumBatchedTokens,
		SpeculativeDecoding:  model.SpeculativeDecoding,
		NumSpeculativeTokens: model.NumSpeculativeTokens,
		CreatedAt:            model.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:            model.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	WriteJSON(w, http.StatusOK, response)
}
