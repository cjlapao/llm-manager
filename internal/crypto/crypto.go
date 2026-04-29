// Package crypto provides encryption utilities for sensitive values stored in the database.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/llm-manager/internal/database"
	"gopkg.in/yaml.v3"
)

const aesPrefix = "$aes$"
const bcryptPrefix = "$bcrypt$"

// encryptionKey returns the AES-256 encryption key with priority:
// 1. LLM_MANAGER_ENCRYPTION_KEY env var
// 2. LLM_MANAGER_ENCRYPTION_KEY in config file
// 3. LLM_MANAGER_ENCRYPTION_KEY in database
// 4. Key file at /opt/ai-server/.llm-manager-key
// It caches the result after the first call.
var cachedKey []byte
var cachedKeyErr error

func encryptionKey() (key []byte, keyErr error) {
	if cachedKey != nil {
		return cachedKey, cachedKeyErr
	}
	defer func() {
		cachedKey = key
		cachedKeyErr = keyErr
	}()

	// 1. Try env var first
	keyStr := os.Getenv("LLM_MANAGER_ENCRYPTION_KEY")
	if keyStr != "" {
		return validateKey(keyStr, "env var")
	}

	// 2. Try config file
	if val, ok := getConfigFileKey(); ok {
		return validateKey(val, "config file")
	}

	// 3. Try database
	if val, ok := getDBKey(); ok {
		return validateKey(val, "database")
	}

	// 4. Fall back to key file
	b, err := os.ReadFile("/opt/ai-server/.llm-manager-key")
	if err == nil {
		key := strings.TrimSpace(string(b))
		if key != "" {
			return validateKey(key, "key file")
		}
	}

	cachedKeyErr = fmt.Errorf(
		"encryption key not found. Set it via one of:\n"+
			"  1. Env var:        LLM_MANAGER_ENCRYPTION_KEY=...\\n"+
			"  2. Config file:    LLM_MANAGER_ENCRYPTION_KEY=... in ~/.config/llm-manager/config.yaml\\n"+
			"  3. Database:       llm-manager config set LLM_MANAGER_ENCRYPTION_KEY <key>\\n"+
			"  4. Key file:       echo <key> > /opt/ai-server/.llm-manager-key",
	)
	return nil, cachedKeyErr
}

// validateKey decodes and validates a base64-encoded 32-byte key.
func validateKey(keyStr, source string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		cachedKeyErr = fmt.Errorf("LLM_MANAGER_ENCRYPTION_KEY from %s must be base64-encoded: %w", source, err)
		return nil, cachedKeyErr
	}
	if len(decoded) != 32 {
		cachedKeyErr = fmt.Errorf("LLM_MANAGER_ENCRYPTION_KEY from %s must be 32 bytes (256 bits), got %d", source, len(decoded))
		return nil, cachedKeyErr
	}
	cachedKey = decoded
	return decoded, nil
}

// configFilePath returns the path to the config file.
func configFilePath() string {
	if val := os.Getenv("LLM_MANAGER_CONFIG"); val != "" {
		return val
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".config/llm-manager/config.yaml"
	}
	return filepath.Join(homeDir, ".config", "llm-manager", "config.yaml")
}

// databaseURL returns the path to the database file.
func databaseURL() string {
	if val := os.Getenv("LLM_MANAGER_DATABASE_URL"); val != "" {
		return val
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".local", "share", "llm-manager", "llm-manager.db")
}

// getConfigFileKey reads LLM_MANAGER_ENCRYPTION_KEY from the config file.
func getConfigFileKey() (string, bool) {
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return "", false
	}
	var m map[string]string
	if err := yaml.Unmarshal(data, &m); err != nil {
		return "", false
	}
	val, ok := m["LLM_MANAGER_ENCRYPTION_KEY"]
	return val, ok && val != ""
}

// getDBKey reads LLM_MANAGER_ENCRYPTION_KEY from the database.
func getDBKey() (string, bool) {
	db, err := database.NewDatabaseManager(databaseURL())
	if err != nil {
		return "", false
	}
	defer db.Close()
	if err := db.Open(); err != nil {
		return "", false
	}
	cfg, err := db.GetConfig("LLM_MANAGER_ENCRYPTION_KEY")
	if err != nil || cfg == nil {
		return "", false
	}
	return cfg.Value, true
}

// GenerateKey generates a random 32-byte key and returns it as base64.
// Use this to create a new encryption key.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt encrypts plaintext using AES-256-GCM and prepends the aes prefix.
// If the value already has the aes prefix it is returned as-is.
// If the value has the bcrypt prefix it is treated as legacy and returned unchanged
// (migration will happen on next read).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	if IsEncryptedAES(plaintext) {
		return plaintext, nil
	}

	key, err := encryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return aesPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// IsEncrypted returns true if the value is encrypted (either AES or bcrypt format).
func IsEncrypted(value string) bool {
	return IsEncryptedAES(value) || IsEncryptedBcrypt(value)
}

// IsEncryptedAES returns true if the value has the AES prefix.
func IsEncryptedAES(value string) bool {
	return len(value) > len(aesPrefix) && value[:len(aesPrefix)] == aesPrefix
}

// IsEncryptedBcrypt returns true if the value has the bcrypt prefix.
func IsEncryptedBcrypt(value string) bool {
	return len(value) > len(bcryptPrefix) && value[:len(bcryptPrefix)] == bcryptPrefix
}

// Decrypt decrypts an AES-encrypted value using AES-256-GCM.
// Returns the plaintext, or the original value if not encrypted.
func Decrypt(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	if !IsEncrypted(encrypted) {
		return encrypted, nil
	}

	if IsEncryptedBcrypt(encrypted) {
		// Legacy bcrypt value — cannot decrypt, return as-is
		// The caller should re-encrypt with AES on next write
		return encrypted, nil
	}

	if !IsEncryptedAES(encrypted) {
		return encrypted, nil
	}

	key, err := encryptionKey()
	if err != nil {
		return "", err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted[len(aesPrefix):])
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// DecryptAndVerify checks if an encrypted value matches the provided plaintext.
// Returns true if it matches, false if it doesn't match or is not encrypted.
func DecryptAndVerify(encrypted, plaintext string) (bool, error) {
	if !IsEncrypted(encrypted) {
		return false, nil
	}

	if IsEncryptedBcrypt(encrypted) {
		// Legacy bcrypt — cannot verify without the original hash
		// Treat as non-matching
		return false, nil
	}

	if !IsEncryptedAES(encrypted) {
		return false, nil
	}

	// Decrypt and compare
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		return false, err
	}
	return decrypted == plaintext, nil
}
