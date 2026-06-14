package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/user/llm-manager/internal/database/models"
)

// SpeechHandler handles speech-model REST API endpoints for listing, starting, and stopping
// STT, TTS, and Omni model containers.
type SpeechHandler struct {
	*APIContext
}

// SpeechModelInfo represents a speech model with its container status.
//
//	swag:responseModel
type SpeechModelInfo struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Container string `json:"container"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

// SpeechListResponse is the response shape for GET /api/speech.
//
//	swag:responseModel
type SpeechListResponse struct {
	STTModels  []SpeechModelInfo `json:"stt"`
	TTSModels  []SpeechModelInfo `json:"tts"`
	OmniModels []SpeechModelInfo `json:"omni"`
}

// SpeechStartRequest is the request body for POST /api/speech/start.
//
//	swag:responseModel
type SpeechStartRequest struct {
	Slugs         []string `json:"slugs"`
	AllowMultiple bool     `json:"allowMultiple"`
}

// SpeechStopRequest is the request body for POST /api/speech/stop.
//
//	swag:responseModel
type SpeechStopRequest struct {
	Slugs []string `json:"slugs"`
}

// SpeechStartResponse is the response body for POST /api/speech/start.
// Supports partial-success results — lists both started slugs and error messages.
//
//	swag:responseModel
type SpeechStartResponse struct {
	Started []string `json:"started"`
	Errors  []string `json:"errors,omitempty"`
}

// SpeechStopResponse is the response body for POST /api/speech/stop.
// Supports partial-stop results — lists both stopped slugs and error messages.
//
//	swag:responseModel
type SpeechStopResponse struct {
	Stopped []string `json:"stopped"`
	Errors  []string `json:"errors,omitempty"`
}

// modelsToSpeechInfo converts a slice of models.Model to []SpeechModelInfo with container status.
func modelsToSpeechInfo(h *SpeechHandler, ml []models.Model) []SpeechModelInfo {
	result := make([]SpeechModelInfo, 0, len(ml))
	for _, m := range ml {
		status, err := h.ContainerService.GetModelStatus(m.Slug)
		if err != nil {
			result = append(result, SpeechModelInfo{
				Slug:      m.Slug,
				Name:      m.Name,
				Container: m.Container,
				Port:      m.Port,
				Status:    "unknown",
			})
		} else {
			result = append(result, SpeechModelInfo{
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

// ListSpeech lists all speech models (STT, TTS, Omni) grouped by subtype.
//
// GET /api/speech
//
//	@Summary	List speech models
//	@Description	Lists all speech models (STT, TTS, Omni) grouped by subtype with their container status. OData query parameters ($filter, $orderby, $skip, $top) can be used to filter and paginate within each subtype.
//	@Tags	speech
//	@Accept	json
//	@Produce	json
//	@Success	200	{object}	SpeechListResponse	"Grouped list of speech models by subtype"
//	@Failure	500	{object}	map[string]string	"Internal server error"
//	@Router	/speech [get]
func (h *SpeechHandler) ListSpeech(w http.ResponseWriter, r *http.Request) {
	sttModels, err := h.DB.ListModelsByTypeSubType("speech", "stt")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list STT models: "+err.Error())
		return
	}
	ttsModels, err := h.DB.ListModelsByTypeSubType("speech", "tts")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list TTS models: "+err.Error())
		return
	}
	omniModels, err := h.DB.ListModelsByTypeSubType("speech", "omni")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list Omni models: "+err.Error())
		return
	}

	resp := SpeechListResponse{
		STTModels:  modelsToSpeechInfo(h, sttModels),
		TTSModels:  modelsToSpeechInfo(h, ttsModels),
		OmniModels: modelsToSpeechInfo(h, omniModels),
	}
	WriteJSON(w, http.StatusOK, resp)
}

// StartSpeechAPI starts one or more speech model containers based on the provided slugs.
// Resolves each slug's type from the database and uses the AllowMultiple flag.
// Partial success: failures do not prevent other slugs from being started.
//
// POST /api/speech/start
//
//	@Summary	Start speech models
//	@Description	Starts speech model containers by slug. Each slug's subtype (STT/TTS/Omni) is automatically resolved from the database. The allowMultiple flag controls whether peer containers of the same subtype are stopped before starting. Supports partial success — failed starts do not abort remaining slugs.
//	@Tags	speech
//	@Accept	json
//	@Produce	json
//	@Param	body	body	SpeechStartRequest	true	"Request with slugs and optional allowMultiple flag"
//	@Success	200	{object}	SpeechStartResponse	"List of started slugs and any errors"
//	@Failure	400	{object}	map[string]string	"Invalid request body"
//	@Failure	404	{object}	map[string]string	"Model slug not found in database"
//	@Router	/speech/start [post]
func (h *SpeechHandler) StartSpeechAPI(w http.ResponseWriter, r *http.Request) {
	var req SpeechStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if len(req.Slugs) == 0 {
		WriteError(w, http.StatusBadRequest, "no slugs provided")
		return
	}

	started := make([]string, 0, len(req.Slugs))
	errs := make([]string, 0)

	for _, slug := range req.Slugs {
		model, err := h.DB.GetModel(slug)
		if err != nil {
			errs = append(errs, fmt.Sprintf("model %q not found: %v", slug, err))
			continue
		}
		if model.Type != "speech" {
			errs = append(errs, fmt.Sprintf("model %q is not a speech model (type=%q)", slug, model.Type))
			continue
		}
		if err := h.ContainerService.StartModelWithHealthCheck(slug, req.AllowMultiple); err != nil {
			errs = append(errs, fmt.Sprintf("failed to start %q: %v", slug, err))
			continue
		}
		started = append(started, slug)
	}

	resp := SpeechStartResponse{Started: started, Errors: errs}

	if len(started) == 0 && len(errs) > 0 {
		WriteError(w, http.StatusInternalServerError, strings.Join(errs, "; "))
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

// StopSpeechAPI stops speech model containers. When no slugs are provided, all
// running speech containers across subtypes (STT, TTS, Omni) are stopped.
// Partial failure: errors in individual stops do not prevent others from completing.
//
// POST /api/speech/stop
//
//	@Summary	Stop speech models
//	@Description	Stops specified speech model containers by slug. When the slugs array is empty or omitted, all running speech containers across STT, TTS, and Omni subtypes are stopped. Partial failures do not prevent other stops from completing.
//	@Tags	speech
//	@Accept	json
//	@Produce	json
//	@Param	body	body	SpeechStopRequest	true	"Request with slugs; empty stops all speech containers"
//	@Success	200	{object}	SpeechStopResponse	"List of stopped slugs and any errors"
//	@Failure	400	{object}	map[string]string	"Invalid request body"
//	@Router	/speech/stop [post]
func (h *SpeechHandler) StopSpeechAPI(w http.ResponseWriter, r *http.Request) {
	var req SpeechStopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	stopped := make([]string, 0)
	errs := make([]string, 0)

	speechSubTypes := []string{"stt", "tts", "omni"}

	if len(req.Slugs) == 0 {
		// Stop all speech containers across every subtype
		for _, sub := range speechSubTypes {
			ml, listErr := h.DB.ListModelsByTypeSubType("speech", sub)
			if listErr != nil {
				errs = append(errs, fmt.Sprintf("failed to list %s models: %v", sub, listErr))
				continue
			}
			for _, m := range ml {
				stopErr := h.ContainerService.StopModelBySlug(m.Slug)
				if stopErr != nil {
					errs = append(errs, fmt.Sprintf("failed to stop %q: %v", m.Slug, stopErr))
				} else {
					stopped = append(stopped, m.Slug)
				}
			}
		}
	} else {
		// Stop the specific slugs named in the request
		for _, slug := range req.Slugs {
			stopErr := h.ContainerService.StopModelBySlug(slug)
			if stopErr != nil {
				errs = append(errs, fmt.Sprintf("failed to stop %q: %v", slug, stopErr))
			} else {
				stopped = append(stopped, slug)
			}
		}
	}

	resp := SpeechStopResponse{Stopped: stopped, Errors: errs}

	if len(stopped) == 0 && len(errs) > 0 {
		WriteError(w, http.StatusInternalServerError, strings.Join(errs, "; "))
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}
