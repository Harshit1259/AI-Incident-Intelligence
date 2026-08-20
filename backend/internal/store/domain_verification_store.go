package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// DomainVerificationStore manages DNS domain ownership records.
type DomainVerificationStore struct {
	db *sql.DB
}

// NewDomainVerificationStore returns a new DomainVerificationStore.
func NewDomainVerificationStore(db *sql.DB) *DomainVerificationStore {
	return &DomainVerificationStore{db: db}
}

// Create inserts a new domain verification record.
func (s *DomainVerificationStore) Create(dv models.DomainVerification) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO domain_verifications (
			id, tenant_id, domain, verified, verification_token,
			dns_txt_record, verified_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		dv.ID, dv.TenantID, dv.Domain, dv.Verified,
		dv.VerificationToken, dv.DNSTXTRecord,
		dv.VerifiedAt, now, now,
	)
	return err
}

// GetByID returns the domain verification record with the given id scoped to tenantID.
func (s *DomainVerificationStore) GetByID(id, tenantID string) (*models.DomainVerification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, domain, verified, verification_token,
		       dns_txt_record, verified_at, created_at, updated_at
		FROM domain_verifications
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanDomainVerification(row)
}

// GetByDomain returns the domain verification record for a specific domain string.
func (s *DomainVerificationStore) GetByDomain(domain string) (*models.DomainVerification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, domain, verified, verification_token,
		       dns_txt_record, verified_at, created_at, updated_at
		FROM domain_verifications
		WHERE domain = $1`,
		domain,
	)
	return scanDomainVerification(row)
}

// GetTenantForDomain returns the tenantID for a verified domain, or empty string if not found.
func (s *DomainVerificationStore) GetTenantForDomain(domain string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var tenantID string
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id FROM domain_verifications
		WHERE domain = $1 AND verified = true`,
		domain,
	).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return tenantID, err
}

// ListByTenant returns all domain verification records for a tenant.
func (s *DomainVerificationStore) ListByTenant(tenantID string) ([]models.DomainVerification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, domain, verified, verification_token,
		       dns_txt_record, verified_at, created_at, updated_at
		FROM domain_verifications
		WHERE tenant_id = $1
		ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DomainVerification
	for rows.Next() {
		dv, err := scanDomainVerification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *dv)
	}
	return out, rows.Err()
}

// MarkVerified sets verified=true and records the verification timestamp.
func (s *DomainVerificationStore) MarkVerified(id, tenantID string, verifiedAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE domain_verifications
		SET verified = true, verified_at = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3`,
		verifiedAt, id, tenantID,
	)
	return err
}

// Delete removes a domain verification record.
func (s *DomainVerificationStore) Delete(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM domain_verifications WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// DeleteByDomain removes a domain verification record by domain string.
func (s *DomainVerificationStore) DeleteByDomain(domain, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM domain_verifications WHERE domain = $1 AND tenant_id = $2`,
		domain, tenantID,
	)
	return err
}

// ── scan helper ───────────────────────────────────────────────────────────────

func scanDomainVerification(row interface {
	Scan(...interface{}) error
}) (*models.DomainVerification, error) {
	var dv models.DomainVerification
	err := row.Scan(
		&dv.ID, &dv.TenantID, &dv.Domain, &dv.Verified,
		&dv.VerificationToken, &dv.DNSTXTRecord,
		&dv.VerifiedAt, &dv.CreatedAt, &dv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &dv, nil
}
