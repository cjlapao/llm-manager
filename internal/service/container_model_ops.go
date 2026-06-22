package service

import (
	"fmt"

	"github.com/user/llm-manager/internal/database/models"
)

// EstimateMemory returns a MemoryResult for a model based on its profile data.
// Returns nil and no error if the model has no profile data.
func (s *ContainerService) EstimateMemory(slug string) (*MemoryResult, error) {
	model, err := s.db.GetModel(slug)
	if err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	return EstimateMemory(model)
}

// ListRAGEmbeddingModels returns all RAG embedding models from the database.
func (s *ContainerService) ListRAGEmbeddingModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("rag", "embedding")
}

// ListRAGRerankerModels returns all RAG reranker models from the database.
func (s *ContainerService) ListRAGRerankerModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("rag", "reranker")
}

// ListSpeechModels returns all speech models from the database (across all subtypes).
// It queries each known speech subtype individually and merges the results.
func (s *ContainerService) ListSpeechModels() ([]models.Model, error) {
	var all []models.Model
	for _, subType := range []string{"stt", "tts", "omni"} {
		mdlList, err := s.db.ListModelsByTypeSubType("speech", subType)
		if err != nil {
			return nil, fmt.Errorf("failed to list speech/%s models: %w", subType, err)
		}
		all = append(all, mdlList...)
	}
	return all, nil
}

// ListSTTModels returns all speech STT models from the database.
func (s *ContainerService) ListSTTModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "stt")
}

// ListTTSModels returns all speech TTS models from the database.
func (s *ContainerService) ListTTSModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "tts")
}

// ListOmniModels returns all speech omni models from the database.
func (s *ContainerService) ListOmniModels() ([]models.Model, error) {
	return s.db.ListModelsByTypeSubType("speech", "omni")
}
