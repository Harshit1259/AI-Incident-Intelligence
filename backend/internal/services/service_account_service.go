package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// ServiceAccountService manages machine identity lifecycle.
type ServiceAccountService struct {
	saStore *store.ServiceAccountStore
}

// NewServiceAccountService creates a new ServiceAccountService.
func NewServiceAccountService(saStore *store.ServiceAccountStore) *ServiceAccountService {
	return &ServiceAccountService{saStore: saStore}
}

// Create creates a new service account.
func (s *ServiceAccountService) Create(tenantID, name, description, role, createdBy string) (*models.ServiceAccount, error) {
	if name == "" {
		return nil, fmt.Errorf("service_account: name is required")
	}
	if !models.ValidRole(role) {
		role = models.RoleOperator // default
	}

	sa := models.ServiceAccount{
		ID:          generateID(),
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		Role:        role,
		Enabled:     true,
		CreatedBy:   createdBy,
	}
	if err := s.saStore.Create(sa); err != nil {
		return nil, fmt.Errorf("service_account: create: %w", err)
	}
	return &sa, nil
}

// List returns all service accounts for a tenant.
func (s *ServiceAccountService) List(tenantID string) ([]models.ServiceAccount, error) {
	return s.saStore.List(tenantID)
}

// GetByID returns a service account by id.
func (s *ServiceAccountService) GetByID(id, tenantID string) (*models.ServiceAccount, error) {
	sa, err := s.saStore.GetByID(id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("service_account: not found: %w", err)
	}
	return sa, nil
}

// SetEnabled enables or disables a service account.
func (s *ServiceAccountService) SetEnabled(id, tenantID string, enabled bool) error {
	return s.saStore.SetEnabled(id, tenantID, enabled)
}

// Delete removes a service account.
func (s *ServiceAccountService) Delete(id, tenantID string) error {
	return s.saStore.Delete(id, tenantID)
}

// IssueJWT issues a JWT for a service account with is_sa=true.
func (s *ServiceAccountService) IssueJWT(sa models.ServiceAccount, jwtSecret string, ttl time.Duration) (string, error) {
	if !sa.Enabled {
		return "", fmt.Errorf("service_account: account %q is disabled", sa.ID)
	}

	now := time.Now().Unix()
	claims := models.TokenClaims{
		UserID:        sa.ID,
		TenantID:      sa.TenantID,
		Role:          sa.Role,
		IsServiceAcct: true,
		Exp:           now + int64(ttl.Seconds()),
		Iat:           now,
	}

	token, err := middleware.GenerateToken(claims, jwtSecret)
	if err != nil {
		return "", fmt.Errorf("service_account: jwt issue: %w", err)
	}

	// Update last_used_at asynchronously.
	go func() { _ = s.saStore.UpdateLastUsed(sa.ID, time.Now().UTC()) }()

	return token, nil
}

// IssueJWTWithScopes issues a scoped JWT for a service account.
func (s *ServiceAccountService) IssueJWTWithScopes(sa models.ServiceAccount, jwtSecret string, ttl time.Duration, scopes []string) (string, error) {
	if !sa.Enabled {
		return "", fmt.Errorf("service_account: account %q is disabled", sa.ID)
	}

	now := time.Now().Unix()
	claims := models.TokenClaims{
		UserID:        sa.ID,
		TenantID:      sa.TenantID,
		Role:          sa.Role,
		IsServiceAcct: true,
		Scopes:        scopes,
		Exp:           now + int64(ttl.Seconds()),
		Iat:           now,
	}

	token, err := middleware.GenerateToken(claims, jwtSecret)
	if err != nil {
		return "", fmt.Errorf("service_account: jwt issue: %w", err)
	}

	go func() { _ = s.saStore.UpdateLastUsed(sa.ID, time.Now().UTC()) }()

	return token, nil
}
