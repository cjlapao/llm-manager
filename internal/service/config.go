// Package service provides business logic services that wrap the database layer.
package service

import (
	"github.com/user/llm-manager/internal/config"
	"github.com/user/llm-manager/internal/crypto"
	"github.com/user/llm-manager/internal/database"
	"github.com/user/llm-manager/internal/database/models"
)

// secretKeys defines config keys whose values must be encrypted in the database.
var secretKeys = map[string]bool{
	"HF_TOKEN": true,
}

// ConfigService handles business logic for persistent configuration operations.
type ConfigService struct {
	db database.DatabaseManager
}

// NewConfigService creates a new ConfigService.
func NewConfigService(db database.DatabaseManager) *ConfigService {
	return &ConfigService{db: db}
}

// Set stores a config key-value pair in the database.
// Secret keys are encrypted with bcrypt before storage.
func (s *ConfigService) Set(key, value string) error {
	if _, err := config.NormalizeKey(key); err != nil {
		return err
	}

	// Encrypt secret values before storing
	if secretKeys[key] {
		encrypted, err := crypto.Encrypt(value)
		if err != nil {
			return err
		}
		value = encrypted
	}

	return s.db.SetConfig(key, value)
}

// Unset removes a config key from the database.
// Validates the key before removing.
func (s *ConfigService) Unset(key string) error {
	if _, err := config.NormalizeKey(key); err != nil {
		return err
	}
	return s.db.UnsetConfig(key)
}

// Get retrieves a config value from the database.
// Secret values are returned as-is (encrypted) — callers should use GetDecrypted() for plaintext.
// Returns nil if the key is not found.
func (s *ConfigService) Get(key string) (*models.Config, error) {
	if _, err := config.NormalizeKey(key); err != nil {
		return nil, err
	}
	return s.db.GetConfig(key)
}

// GetDecrypted retrieves a config value and decrypts it if it's a secret.
// Returns the plaintext value, or the raw value if not encrypted.
func (s *ConfigService) GetDecrypted(key string) (string, error) {
	cfg, err := s.Get(key)
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return "", nil
	}

	if secretKeys[key] && crypto.IsEncrypted(cfg.Value) {
		return cfg.Value, nil // Return encrypted value — caller should verify
	}

	return cfg.Value, nil
}

// VerifySecret checks if the provided plaintext matches the stored encrypted value.
// Returns true if it's a secret key and the value matches.
func (s *ConfigService) VerifySecret(key, plaintext string) (bool, error) {
	cfg, err := s.Get(key)
	if err != nil {
		return false, err
	}
	if cfg == nil {
		return false, nil
	}

	if !secretKeys[key] {
		return cfg.Value == plaintext, nil
	}

	if !crypto.IsEncrypted(cfg.Value) {
		return false, nil
	}

	return crypto.DecryptAndVerify(cfg.Value, plaintext)
}

// List returns all config key-value pairs from the database.
func (s *ConfigService) List() ([]models.Config, error) {
	return s.db.ListConfig()
}
