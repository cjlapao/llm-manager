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

// ListModels handles GET /api/models — returns all models as a JSON array.
func (h *ModelHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.ModelService.ListModels()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list models: "+err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, models)
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

	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return
	}

	if err := h.ModelService.UpdateModel(slug, updates); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update model: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "model updated"})
}

// DeleteModel handles DELETE /api/models/{slug} — deletes a model by slug.
func (h *ModelHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if err := h.ModelService.DeleteModel(slug); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete model: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ModelInfoResponse is the enriched response shape for GET /api/models/{slug}/info.
// It includes the model record with JSON string fields parsed into typed objects.
type ModelInfoResponse struct {
	Slug            string                 `json:"slug"`
	Type            string                 `json:"type"`
	SubType         string                 `json:"sub_type"`
	Name            string                 `json:"name"`
	HFRepo          string                 `json:"hf_repo"`
	Container       string                 `json:"container"`
	Port            int                    `json:"port"`
	EngineType      string                 `json:"engine_type"`
	InputTokenCost  float64                `json:"input_token_cost"`
	OutputTokenCost float64                `json:"output_token_cost"`
	Capabilities    []string               `json:"capabilities"`
	LiteLLMParams   map[string]interface{} `json:"litellm_params"`
	ModelInfo       map[string]interface{} `json:"model_info"`
	Default         bool                   `json:"default"`
	CreatedAt       string                 `json:"created_at"`
	UpdatedAt       string                 `json:"updated_at"`
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
		Slug:            model.Slug,
		Type:            model.Type,
		SubType:         model.SubType,
		Name:            model.Name,
		HFRepo:          model.HFRepo,
		Container:       model.Container,
		Port:            model.Port,
		EngineType:      model.EngineType,
		InputTokenCost:  model.InputTokenCost,
		OutputTokenCost: model.OutputTokenCost,
		Capabilities:    capabilities,
		LiteLLMParams:   liteLLMParams,
		ModelInfo:       modelInfo,
		Default:         model.Default,
		CreatedAt:       model.CreatedAt.String(),
		UpdatedAt:       model.UpdatedAt.String(),
	}

	WriteJSON(w, http.StatusOK, response)
}
