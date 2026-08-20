// Package crypto provides per-tenant AES-256-GCM field-level encryption.
//
// Key derivation: SHA-256(masterKey || ":" || tenantID || ":" || "aiops-v1")
// This produces a 256-bit tenant-specific key without requiring an external KMS.
// When masterKey is empty, Encrypt/Decrypt are transparent no-ops.
//
// Encrypted envelope layout: [12-byte GCM nonce][ciphertext+tag]
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Encryptor manages per-tenant AES-256-GCM encryption.
type Encryptor struct {
	masterKey []byte     // raw 32 bytes; nil = disabled
	keyCache  sync.Map   // tenantID -> []byte (derived key)
}

// New creates an Encryptor from a hex-encoded master key.
// Pass an empty string to disable encryption (passthrough mode).
func New(masterKeyHex string) (*Encryptor, error) {
	e := &Encryptor{}
	if masterKeyHex == "" {
		return e, nil // passthrough mode
	}
	key, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("crypto: invalid master key hex: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: master key must be 32 bytes (64 hex chars), got %d", len(key))
	}
	e.masterKey = key
	return e, nil
}

// IsEnabled reports whether field encryption is active.
func (e *Encryptor) IsEnabled() bool { return len(e.masterKey) == 32 }

// Encrypt encrypts plaintext for a specific tenant.
// Returns plaintext unchanged when encryption is disabled.
func (e *Encryptor) Encrypt(tenantID string, plaintext []byte) ([]byte, error) {
	if !e.IsEnabled() {
		return plaintext, nil
	}
	key := e.deriveKey(tenantID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts ciphertext for a specific tenant.
// Returns ciphertext unchanged when encryption is disabled.
func (e *Encryptor) Decrypt(tenantID string, ciphertext []byte) ([]byte, error) {
	if !e.IsEnabled() {
		return ciphertext, nil
	}
	key := e.deriveKey(tenantID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("crypto: ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

// EncryptString encrypts a string value.
func (e *Encryptor) EncryptString(tenantID, plaintext string) (string, error) {
	ct, err := e.Encrypt(tenantID, []byte(plaintext))
	if err != nil {
		return "", err
	}
	if !e.IsEnabled() {
		return plaintext, nil
	}
	return hex.EncodeToString(ct), nil
}

// DecryptString decrypts a hex-encoded encrypted string.
func (e *Encryptor) DecryptString(tenantID, ciphertext string) (string, error) {
	if !e.IsEnabled() {
		return ciphertext, nil
	}
	ct, err := hex.DecodeString(ciphertext)
	if err != nil {
		return ciphertext, nil // best-effort: return as-is if not encrypted
	}
	plain, err := e.Decrypt(tenantID, ct)
	if err != nil {
		return ciphertext, nil // best-effort: return as-is on decryption failure
	}
	return string(plain), nil
}

// deriveKey returns the per-tenant AES-256 key, caching the derivation.
func (e *Encryptor) deriveKey(tenantID string) []byte {
	if v, ok := e.keyCache.Load(tenantID); ok {
		return v.([]byte)
	}
	h := sha256.New()
	h.Write(e.masterKey)
	h.Write([]byte(":"))
	h.Write([]byte(tenantID))
	h.Write([]byte(":aiops-v1"))
	key := h.Sum(nil) // 32 bytes
	e.keyCache.Store(tenantID, key)
	return key
}
