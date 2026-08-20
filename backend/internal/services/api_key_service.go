package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// APIKeyService manages API key lifecycle: create, validate, revoke, rotate.
type APIKeyService struct {
	keyStore *store.APIKeyStore
	saStore  *store.ServiceAccountStore
}

// NewAPIKeyService creates a new APIKeyService.
func NewAPIKeyService(keyStore *store.APIKeyStore, saStore *store.ServiceAccountStore) *APIKeyService {
	return &APIKeyService{keyStore: keyStore, saStore: saStore}
}

// rawKeyPrefix is the static prefix for all generated API keys.
const rawKeyPrefix = "aiops_"

// Create generates a new API key, stores only its hash, and returns the full key
// once (RawKey is set). The caller must show RawKey to the user immediately.
func (s *APIKeyService) Create(
	tenantID, ownerID string,
	ownerType models.APIKeyOwnerType,
	name string,
	scopes []string,
	expiresAt *time.Time,
	createdBy string,
) (*models.APIKey, error) {
	if name == "" {
		return nil, errors.New("api_key: name is required")
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("api_key: rand: %w", err)
	}
	rawKey := rawKeyPrefix + hex.EncodeToString(rawBytes)
	keyHash := hashKey(rawKey)
	keyPrefix := rawKey[:12] // first 12 chars: "aiops_" + 6 hex

	k := models.APIKey{
		ID:        generateID(),
		TenantID:  tenantID,
		Name:      name,
		KeyPrefix: keyPrefix,
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		Revoked:   false,
		CreatedBy: createdBy,
		RawKey:    rawKey, // shown once, not stored
	}

	if err := s.keyStore.Create(k, keyHash); err != nil {
		return nil, fmt.Errorf("api_key: create: %w", err)
	}
	return &k, nil
}

// Validate looks up an API key by its raw value. It verifies the key is not
// revoked, not expired, updates last_used_at, and returns the APIKey record.
func (s *APIKeyService) Validate(rawKey string) (*models.APIKey, error) {
	if rawKey == "" {
		return nil, errors.New("api_key: empty key")
	}

	hash := hashKey(rawKey)
	k, err := s.keyStore.GetByHash(hash)
	if err != nil {
		return nil, errors.New("api_key: invalid key")
	}

	if k.Revoked {
		return nil, errors.New("api_key: key has been revoked")
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, errors.New("api_key: key has expired")
	}

	// Update last_used_at in background.
	go func() { _ = s.keyStore.UpdateLastUsed(k.ID, time.Now().UTC()) }()

	return k, nil
}

// Revoke marks an API key as revoked.
func (s *APIKeyService) Revoke(id, tenantID, revokedBy string) error {
	return s.keyStore.Revoke(id, tenantID, revokedBy)
}

// Rotate revokes an existing key and creates a new one with the same metadata.
// Returns the new APIKey with RawKey set (shown once).
func (s *APIKeyService) Rotate(id, tenantID, createdBy string) (*models.APIKey, error) {
	existing, err := s.keyStore.GetByID(id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("api_key: rotate: not found: %w", err)
	}
	if existing.Revoked {
		return nil, errors.New("api_key: rotate: key already revoked")
	}

	// Revoke the old key.
	if err := s.keyStore.Revoke(id, tenantID, createdBy); err != nil {
		return nil, fmt.Errorf("api_key: rotate: revoke old: %w", err)
	}

	// Create a new key with the same metadata.
	return s.Create(
		tenantID, existing.OwnerID, existing.OwnerType,
		existing.Name, existing.Scopes, existing.ExpiresAt, createdBy,
	)
}

// List returns all API keys for a tenant (without raw key material).
func (s *APIKeyService) List(tenantID string) ([]models.APIKey, error) {
	return s.keyStore.ListByTenant(tenantID)
}

// ListByOwner returns all API keys for a specific owner.
func (s *APIKeyService) ListByOwner(tenantID, ownerID string) ([]models.APIKey, error) {
	return s.keyStore.ListByOwner(tenantID, ownerID)
}

// GetByID returns a single API key by id.
func (s *APIKeyService) GetByID(id, tenantID string) (*models.APIKey, error) {
	return s.keyStore.GetByID(id, tenantID)
}

// hashKey computes the hex-encoded SHA-256 hash of a raw API key.
func hashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
