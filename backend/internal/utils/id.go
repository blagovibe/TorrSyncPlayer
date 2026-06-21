// Package utils provides shared utility functions.
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateID generates a unique hex-encoded identifier of the specified byte length.
// Returns an error if random bytes could not be obtained.
func GenerateID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("error reading random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
