package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// SSOStore manages SSO provider configurations and in-flight PKCE state.
type SSOStore struct {
	db *sql.DB
}

// NewSSOStore creates a new SSOStore backed by db.
func NewSSOStore(db *sql.DB) *SSOStore {
	return &SSOStore{db: db}
}

// ── SSO Providers ─────────────────────────────────────────────────────────────

// CreateProvider inserts a new SSO provider record.
func (s *SSOStore) CreateProvider(p models.SSOProvider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	attrJSON, err := marshalStringMap(p.AttributeMapping)
	if err != nil {
		return fmt.Errorf("sso_store create: marshal attr mapping: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sso_providers (
			id, tenant_id, provider_type, name, enabled,
			client_id, client_secret_enc, discovery_url, scopes,
			idp_entity_id, idp_sso_url, idp_cert_enc, sp_entity_id, acs_url,
			attribute_mapping_json, auto_provision, default_role,
			created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,
			$10,$11,$12,$13,$14,
			$15,$16,$17,
			$18,$19
		)`,
		p.ID, p.TenantID, string(p.ProviderType), p.Name, p.Enabled,
		p.ClientID, p.ClientSecret, p.DiscoveryURL, p.Scopes,
		p.IDPEntityID, p.IDPSSOUrl, p.IDPCert, p.SPEntityID, p.ACSURL,
		attrJSON, p.AutoProvision, p.DefaultRole,
		now, now,
	)
	return err
}

// GetProvider returns the SSO provider with the given id scoped to tenantID.
func (s *SSOStore) GetProvider(id, tenantID string) (*models.SSOProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, provider_type, name, enabled,
		       client_id, client_secret_enc, discovery_url, scopes,
		       idp_entity_id, idp_sso_url, idp_cert_enc, sp_entity_id, acs_url,
		       attribute_mapping_json, auto_provision, default_role,
		       created_at, updated_at
		FROM sso_providers
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanProvider(row)
}

// GetProviderByID returns any provider by id (used during public callback flows
// where tenantID is derived from state).
func (s *SSOStore) GetProviderByID(id string) (*models.SSOProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, provider_type, name, enabled,
		       client_id, client_secret_enc, discovery_url, scopes,
		       idp_entity_id, idp_sso_url, idp_cert_enc, sp_entity_id, acs_url,
		       attribute_mapping_json, auto_provision, default_role,
		       created_at, updated_at
		FROM sso_providers
		WHERE id = $1`,
		id,
	)
	return scanProvider(row)
}

// ListProviders returns all SSO providers for a tenant.
func (s *SSOStore) ListProviders(tenantID string) ([]models.SSOProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, provider_type, name, enabled,
		       client_id, client_secret_enc, discovery_url, scopes,
		       idp_entity_id, idp_sso_url, idp_cert_enc, sp_entity_id, acs_url,
		       attribute_mapping_json, auto_provision, default_role,
		       created_at, updated_at
		FROM sso_providers
		WHERE tenant_id = $1
		ORDER BY created_at ASC LIMIT 50`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SSOProvider
	for rows.Next() {
		p, err := scanProviderRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdateProvider updates a mutable SSO provider record.
func (s *SSOStore) UpdateProvider(p models.SSOProvider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	attrJSON, err := marshalStringMap(p.AttributeMapping)
	if err != nil {
		return fmt.Errorf("sso_store update: marshal attr mapping: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE sso_providers SET
			provider_type = $1, name = $2, enabled = $3,
			client_id = $4, client_secret_enc = $5, discovery_url = $6, scopes = $7,
			idp_entity_id = $8, idp_sso_url = $9, idp_cert_enc = $10,
			sp_entity_id = $11, acs_url = $12,
			attribute_mapping_json = $13, auto_provision = $14, default_role = $15,
			updated_at = NOW()
		WHERE id = $16 AND tenant_id = $17`,
		string(p.ProviderType), p.Name, p.Enabled,
		p.ClientID, p.ClientSecret, p.DiscoveryURL, p.Scopes,
		p.IDPEntityID, p.IDPSSOUrl, p.IDPCert,
		p.SPEntityID, p.ACSURL,
		attrJSON, p.AutoProvision, p.DefaultRole,
		p.ID, p.TenantID,
	)
	return err
}

// DeleteProvider removes an SSO provider record.
func (s *SSOStore) DeleteProvider(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM sso_providers WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// GetEnabledProviderForTenant returns the first enabled provider of any type for a tenant.
func (s *SSOStore) GetEnabledProviderForTenant(tenantID string) (*models.SSOProvider, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, provider_type, name, enabled,
		       client_id, client_secret_enc, discovery_url, scopes,
		       idp_entity_id, idp_sso_url, idp_cert_enc, sp_entity_id, acs_url,
		       attribute_mapping_json, auto_provision, default_role,
		       created_at, updated_at
		FROM sso_providers
		WHERE tenant_id = $1 AND enabled = true
		ORDER BY created_at ASC
		LIMIT 1`,
		tenantID,
	)
	return scanProvider(row)
}

// ── SSO State (PKCE) ──────────────────────────────────────────────────────────

// SaveState persists a short-lived OIDC PKCE state.
func (s *SSOStore) SaveState(st models.SSOState) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sso_state (state, tenant_id, provider_id, code_verifier, redirect_to, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())`,
		st.State, st.TenantID, st.ProviderID, st.CodeVerifier, st.RedirectTo, st.ExpiresAt,
	)
	return err
}

// GetState retrieves and validates an unexpired PKCE state record.
// Returns sql.ErrNoRows if not found or expired.
func (s *SSOStore) GetState(state string) (*models.SSOState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT state, tenant_id, provider_id, code_verifier, redirect_to, expires_at
		FROM sso_state
		WHERE state = $1 AND expires_at > NOW()`,
		state,
	)
	var st models.SSOState
	if err := row.Scan(
		&st.State, &st.TenantID, &st.ProviderID,
		&st.CodeVerifier, &st.RedirectTo, &st.ExpiresAt,
	); err != nil {
		return nil, err
	}
	return &st, nil
}

// DeleteState removes a PKCE state record (after use or expiry).
func (s *SSOStore) DeleteState(state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM sso_state WHERE state = $1`, state)
	return err
}

// DeleteExpiredStates cleans up all expired state records.
func (s *SSOStore) DeleteExpiredStates() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM sso_state WHERE expires_at <= NOW()`)
	return err
}

// ── internal scan helpers ─────────────────────────────────────────────────────

type providerScanner interface {
	Scan(dest ...interface{}) error
}

func scanProvider(row providerScanner) (*models.SSOProvider, error) {
	return scanProviderRow(row)
}

func scanProviderRow(row providerScanner) (*models.SSOProvider, error) {
	var p models.SSOProvider
	var attrJSON string
	var pt string
	err := row.Scan(
		&p.ID, &p.TenantID, &pt, &p.Name, &p.Enabled,
		&p.ClientID, &p.ClientSecret, &p.DiscoveryURL, &p.Scopes,
		&p.IDPEntityID, &p.IDPSSOUrl, &p.IDPCert, &p.SPEntityID, &p.ACSURL,
		&attrJSON, &p.AutoProvision, &p.DefaultRole,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.ProviderType = models.SSOProviderType(pt)
	if err := unmarshalStringMap(attrJSON, &p.AttributeMapping); err != nil {
		return nil, fmt.Errorf("sso_store: unmarshal attr mapping: %w", err)
	}
	return &p, nil
}
