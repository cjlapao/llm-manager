package service

import (
	"encoding/json"
	"fmt"
)

// GetModelInfo retrieves detailed information for a model using its slug name.
func (s *LiteLLMService) GetModelInfo(slug string) (*LiteLLMModelInfo, error) {
	dbModel, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve slug %s: %w", slug, err)
	}
	litellmID := dbModel.LitellmModelID
	if litellmID == "" {
		return nil, fmt.Errorf("cannot query %s — no litellm_model_id; run 'litellm add %s' first", slug, slug)
	}
	apiInfo, err := s.GetModelInfoByUUID(litellmID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	if dbModel.LiteLLMParams != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(dbModel.LiteLLMParams), &params); err == nil {
			apiInfo.LiteLLMParams = params
		}
	}
	if dbModel.ModelInfo != "" {
		var minfo map[string]interface{}
		if err := json.Unmarshal([]byte(dbModel.ModelInfo), &minfo); err == nil {
			apiInfo.ModelInfo = minfo
		}
	}
	return apiInfo, nil
}

// GetModelInfoByUUID retrieves detailed information for a specific model using its
// LiteLLM proxy internal row UUID.
func (s *LiteLLMService) GetModelInfoByUUID(litellmID string) (*LiteLLMModelInfo, error) {
	path := fmt.Sprintf("/model/info?litellm_model_id=%s", litellmID)
	body, err := s.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %w", err)
	}
	var result struct {
		Data []LiteLLMModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no model found for litellm id %s", litellmID)
	}
	info := result.Data[0]
	return &info, nil
}
