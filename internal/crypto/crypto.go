// Package crypto provides encryption utilities for sensitive values stored in the database.
package crypto

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const prefix = "$bcrypt$"

// Encrypt hashes a plaintext value using bcrypt and prepends a marker prefix.
// The prefix allows us to distinguish encrypted values from plaintext.
func Encrypt(plaintext string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt encrypt: %w", err)
	}
	return prefix + string(hashed), nil
}

// IsEncrypted checks if a stored value is encrypted (has the bcrypt prefix).
func IsEncrypted(value string) bool {
	return len(value) > len(prefix) && value[:len(prefix)] == prefix
}

// DecryptAndVerify checks if an encrypted value matches the provided plaintext.
// Returns true if the value matches, false if it doesn't match or is not encrypted.
// Returns an error only for invalid bcrypt format.
func DecryptAndVerify(encrypted, plaintext string) (bool, error) {
	if !IsEncrypted(encrypted) {
		return false, nil
	}

	hashed := []byte(encrypted[len(prefix):])
	err := bcrypt.CompareHashAndPassword(hashed, []byte(plaintext))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, fmt.Errorf("bcrypt decrypt: %w", err)
	}
	return true, nil
}
