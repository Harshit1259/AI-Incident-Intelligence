package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// DomainVerificationService manages DNS domain verification for SSO routing.
type DomainVerificationService struct {
	domainStore *store.DomainVerificationStore
	ssoStore    *store.SSOStore
}

// NewDomainVerificationService returns a new DomainVerificationService.
func NewDomainVerificationService(
	domainStore *store.DomainVerificationStore,
	ssoStore *store.SSOStore,
) *DomainVerificationService {
	return &DomainVerificationService{
		domainStore: domainStore,
		ssoStore:    ssoStore,
	}
}

// InitiateVerification creates a domain verification record with a DNS challenge token.
// Returns the DomainVerification containing the DNS TXT record to add.
func (s *DomainVerificationService) InitiateVerification(tenantID, domain string) (*models.DomainVerification, error) {
	// Normalise domain (lowercase, strip trailing dot).
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if domain == "" {
		return nil, fmt.Errorf("domain_verification: empty domain")
	}

	// Check if domain already exists for another tenant.
	existing, err := s.domainStore.GetByDomain(domain)
	if err == nil && existing != nil && existing.TenantID != tenantID {
		return nil, fmt.Errorf("domain_verification: domain %q is claimed by another tenant", domain)
	}

	// Generate a 32-byte random token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("domain_verification: token generate: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	dnsTXTRecord := "aiops-site-verification=" + token

	dv := models.DomainVerification{
		ID:                generateID(),
		TenantID:          tenantID,
		Domain:            domain,
		Verified:          false,
		VerificationToken: token,
		DNSTXTRecord:      dnsTXTRecord,
	}

	// Upsert: delete any existing record for this tenant+domain first.
	_ = s.domainStore.DeleteByDomain(domain, tenantID)

	if err := s.domainStore.Create(dv); err != nil {
		return nil, fmt.Errorf("domain_verification: create: %w", err)
	}
	return &dv, nil
}

// VerifyDomain performs a DNS TXT lookup and marks the domain as verified if
// the expected TXT record is found.
func (s *DomainVerificationService) VerifyDomain(tenantID, domain string) (*models.DomainVerification, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))

	dv, err := s.domainStore.GetByDomain(domain)
	if err != nil {
		return nil, fmt.Errorf("domain_verification: not found: %w", err)
	}
	if dv.TenantID != tenantID {
		return nil, fmt.Errorf("domain_verification: domain not owned by this tenant")
	}
	if dv.Verified {
		return dv, nil // already verified
	}

	// Perform DNS TXT lookup.
	txts, lookupErr := net.LookupTXT(domain)
	if lookupErr != nil {
		return nil, fmt.Errorf("domain_verification: DNS lookup failed: %w", lookupErr)
	}

	found := false
	for _, txt := range txts {
		if txt == dv.DNSTXTRecord {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("domain_verification: TXT record not found; expected %q", dv.DNSTXTRecord)
	}

	now := time.Now().UTC()
	if err := s.domainStore.MarkVerified(dv.ID, tenantID, now); err != nil {
		return nil, fmt.Errorf("domain_verification: mark verified: %w", err)
	}
	dv.Verified = true
	dv.VerifiedAt = &now
	return dv, nil
}

// GetTenantForDomain returns the tenantID of the tenant that owns the verified domain.
// Returns empty string if the domain is not verified.
func (s *DomainVerificationService) GetTenantForDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	return s.domainStore.GetTenantForDomain(domain)
}

// LookupSSOByEmail extracts the domain from an email address and looks up the
// enabled SSO provider for the owning tenant.
func (s *DomainVerificationService) LookupSSOByEmail(email string) (*models.SSOProvider, error) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, fmt.Errorf("domain_verification: invalid email %q", email)
	}
	domain := strings.ToLower(parts[1])

	tenantID, err := s.domainStore.GetTenantForDomain(domain)
	if err != nil {
		return nil, fmt.Errorf("domain_verification: domain lookup: %w", err)
	}
	if tenantID == "" {
		return nil, nil // no SSO configured for this domain
	}

	provider, err := s.ssoStore.GetEnabledProviderForTenant(tenantID)
	if err != nil {
		return nil, fmt.Errorf("domain_verification: no enabled SSO for tenant %s: %w", tenantID, err)
	}
	return provider, nil
}

// ListByTenant returns all domain verification records for a tenant.
func (s *DomainVerificationService) ListByTenant(tenantID string) ([]models.DomainVerification, error) {
	return s.domainStore.ListByTenant(tenantID)
}

// DeleteDomain removes a domain record.
func (s *DomainVerificationService) DeleteDomain(tenantID, domain string) error {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	return s.domainStore.DeleteByDomain(domain, tenantID)
}
