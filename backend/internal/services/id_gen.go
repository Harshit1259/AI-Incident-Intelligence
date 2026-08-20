package services

import (
	"crypto/rand"
	"encoding/hex"
)

// generateID produces a random 16-byte hex-encoded identifier.
// Used across identity services to avoid repeating the same pattern.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
