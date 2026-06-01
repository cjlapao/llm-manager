package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/service"
	"gopkg.in/yaml.v3"
)

// ModelUtilHandler provides HTTP handlers for model utility operations:
// import, export, compose generation, and HF cache clearing.
type ModelUtilHandler struct {
	*APIContext
}

// importRequest represents the JSON body for importing a model via YAML content.
//
//	swag:responseModel
type importRequest struct {
	YAMLContent string `json:"yaml_content"`
}

// ImportModel handles POST /api/models/import — imports a model from a YAML file
// or YAML content provided in the request body. Accepts both multipart/form-data
// (file upload) and JSON body with a yaml_content field.
//
//	@Summary	Import a model from YAML
//	@Description	Imports an LLM model definition from a YAML file or YAML content. Accepts multipart/form-data (file upload) or JSON body with a yaml_content field.
//	@Tags		models-utilities
//	@Accept		json
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		yaml_content	body		importRequest	false	"YAML content for import"
//	@Param		file			formData	file		false	"YAML file to upload"
//	@Success	201				{object}	models.Model	"Imported model"
//	@Failure	400				{object}	map[string]string	"Malformed request or invalid file"
//	@Failure	500				{object}	map[string]string	"Internal server error"
//	@Router		/models/import [post]
func (h *ModelUtilHandler) ImportModel(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")

	// Determine whether this is a JSON body or multipart upload
	if strings.HasPrefix(contentType, "application/json") {
		h.importFromJSON(w, r)
		return
	}

	// Default to multipart/form-data for file uploads
	h.importFromFile(w, r)
}

// importFromJSON handles imports where the YAML content is provided in a JSON body.
func (h *ModelUtilHandler) importFromJSON(w http.ResponseWriter, r *http.Request) {
	var req importRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return
	}

	if req.YAMLContent == "" {
		WriteError(w, http.StatusBadRequest, "yaml_content field is required")
		return
	}

	// Write content to a temporary file and call ImportModel
	tmpFile, err := os.CreateTemp("", "model-import-*.yaml")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create temp file: "+err.Error())
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(req.YAMLContent); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write temp file: "+err.Error())
		return
	}
	if err := tmpFile.Close(); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to close temp file: "+err.Error())
		return
	}

	model, err := h.ModelService.ImportModel(tmpFile.Name(), service.ImportOverrides{
		Type:   "llm",
		Engine: "vllm",
	})
	if err != nil {
		WriteError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, model)
}

// importFromFile handles multipart/form-data file upload imports.
func (h *ModelUtilHandler) importFromFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB limit
		WriteError(w, http.StatusBadRequest, "multipart form too large: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing or invalid file field: "+err.Error())
		return
	}
	defer file.Close()

	// Validate file extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".yaml" && ext != ".yml" {
		WriteError(w, http.StatusBadRequest, "file must be a .yaml or .yml file")
		return
	}

	// Write uploaded file to a temporary location
	tmpFile, err := os.CreateTemp("", "model-import-*.yaml")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create temp file: "+err.Error())
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, file); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write temp file: "+err.Error())
		return
	}
	if err := tmpFile.Close(); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to close temp file: "+err.Error())
		return
	}

	model, err := h.ModelService.ImportModel(tmpFile.Name(), service.ImportOverrides{
		Type:   "llm",
		Engine: "vllm",
	})
	if err != nil {
		WriteError(w, http.StatusBadRequest, "import failed: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, model)
}

// ExportModel handles GET /api/models/{slug}/export — exports a model from the
// database as a YAML document suitable for re-import.
//
//	@Summary	Export a model to YAML
//	@Description	Exports an LLM model from the database as a YAML document suitable for re-import to another instance.
//	@Tags		models-utilities
//	@Produce	text/x-yaml
//	@Param		slug	path		string	true	"Model slug"
//	@Success	200		{file}	string	"YAML export document"
//	@Failure	400		{object}	map[string]string	"Missing slug"
//	@Failure	404		{object}	map[string]string	"Model not found"
//	@Failure	500		{object}	map[string]string	"Export or serialization failed"
//	@Router		/models/{slug}/export [get]
func (h *ModelUtilHandler) ExportModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		WriteError(w, http.StatusBadRequest, "slug is required")
		return
	}

	y, err := h.ModelService.ExportModel(slug)
	if err != nil {
		// Distinguish between "not found" and other errors
		if strings.Contains(err.Error(), "not found") {
			WriteError(w, http.StatusNotFound, "model not found: "+slug)
			return
		}
		WriteError(w, http.StatusInternalServerError, "export failed: "+err.Error())
		return
	}

	// Serialize to YAML
	yamlBytes, err := yaml.Marshal(y)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to serialize YAML: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write(yamlBytes)
}

// ComposeModel handles GET /api/models/{slug}/compose — generates a docker-compose
// YAML configuration for the specified model.
//
//	@Summary	Generate docker-compose for a model
//	@Description	Generates a Docker Compose YAML configuration for the specified LLM model, including container settings, resource allocation, and vLLM engine configuration.
//	@Tags		models-utilities
//	@Produce	text/x-yaml
//	@Param		slug	path		string	true	"Model slug"
//	@Success	200		{file}	string	"Generated docker-compose YAML"
//	@Failure	400		{object}	map[string]string	"Missing slug"
//	@Failure	404		{object}	map[string]string	"Model not found"
//	@Failure	500		{object}	map[string]string	"Compose generation failed"
//	@Router		/models/{slug}/compose [get]
func (h *ModelUtilHandler) ComposeModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		WriteError(w, http.StatusBadRequest, "slug is required")
		return
	}

	// Verify model exists in DB first
	if _, err := h.ModelService.GetModel(slug); err != nil {
		WriteError(w, http.StatusNotFound, "model not found: "+slug)
		return
	}

	// Create compose generator
	generator, err := service.NewComposeGenerator()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create compose generator: "+err.Error())
		return
	}

	// Generate compose YAML with default engine config
	composeYAML, err := h.ModelService.GenerateCompose(slug, generator, service.EngineComposeConfig{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to generate compose: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(composeYAML))
}

// ClearCache handles DELETE /api/models/{slug}/cache — removes the HuggingFace
// model weights cache directory for the specified model.
//
//	@Summary	Clear HuggingFace model cache
//	@Description	Removes the HuggingFace model weights cache directory for the specified model. This frees disk space but requires re-downloading on next use.
//	@Tags		models-utilities
//	@Accept		json
//	@Produce	json
//	@Param		slug	path		string	true	"Model slug"
//	@Success	204		"No Content"	"Cache cleared"
//	@Failure	400		{object}	map[string]string	"Missing slug or model has no HF repo"
//	@Failure	404		{object}	map[string]string	"Model not found"
//	@Failure	500		{object}	map[string]string	"Failed to remove cache directory"
//	@Router		/models/{slug}/cache [delete]
func (h *ModelUtilHandler) ClearCache(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]

	if slug == "" {
		WriteError(w, http.StatusBadRequest, "slug is required")
		return
	}

	model, err := h.ModelService.GetModel(slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, "model not found: "+slug)
		return
	}

	if model.HFRepo == "" {
		WriteError(w, http.StatusBadRequest, "model has no HF repo configured")
		return
	}

	// Convert HF repo to cache directory name: Qwen/Qwen3.6-35B-A3B -> models--Qwen--Qwen3.6-35B-A3B
	cacheDir := "models--" + strings.ReplaceAll(model.HFRepo, "/", "--")

	cfg := h.Config
	if cfg == nil {
		cfg = &config.Config{}
	}

	// Check both standard and legacy cache layouts
	cachePaths := []string{
		filepath.Join(cfg.HFCacheDir, "hub", cacheDir),
		filepath.Join(cfg.HFCacheDir, cacheDir),
	}

	var deletedPaths []string
	for _, dir := range cachePaths {
		if _, statErr := os.Stat(dir); statErr == nil {
			if err := os.RemoveAll(dir); err != nil {
				WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to remove cache %s: %v", dir, err))
				return
			}
			deletedPaths = append(deletedPaths, dir)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
