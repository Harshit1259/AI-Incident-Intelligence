package models

import "time"

// ── SSO Provider ──────────────────────────────────────────────────────────────

// SSOProviderType enumerates supported SSO protocol types.
type SSOProviderType string

const (
	SSOTypeOIDC SSOProviderType = "oidc"
	SSOTypeSAML SSOProviderType = "saml"
)

// SSOProvider holds configuration for a single SSO integration (OIDC or SAML).
type SSOProvider struct {
	ID           string          `json:"id"`
	TenantID     string          `json:"tenant_id"`
	ProviderType SSOProviderType `json:"provider_type"`
	Name         string          `json:"name"`
	Enabled      bool            `json:"enabled"`
	// OIDC fields
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"-"` // never serialised to JSON
	DiscoveryURL string `json:"discovery_url"`
	Scopes       string `json:"scopes"`
	// SAML fields
	IDPEntityID string `json:"idp_entity_id,omitempty"`
	IDPSSOUrl   string `json:"idp_sso_url,omitempty"`
	IDPCert     string `json:"-"` // never serialised
	SPEntityID  string `json:"sp_entity_id,omitempty"`
	ACSURL      string `json:"acs_url,omitempty"`
	// Provisioning
	AttributeMapping map[string]string `json:"attribute_mapping,omitempty"`
	AutoProvision    bool              `json:"auto_provision"`
	DefaultRole      string            `json:"default_role"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// SSOState holds short-lived PKCE state for an in-flight OIDC flow.
type SSOState struct {
	State        string    `json:"state"`
	TenantID     string    `json:"tenant_id"`
	ProviderID   string    `json:"provider_id"`
	CodeVerifier string    `json:"-"`
	RedirectTo   string    `json:"redirect_to"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// ── Domain Verification ───────────────────────────────────────────────────────

// DomainVerification represents a domain ownership claim for a tenant.
type DomainVerification struct {
	ID                string     `json:"id"`
	TenantID          string     `json:"tenant_id"`
	Domain            string     `json:"domain"`
	Verified          bool       `json:"verified"`
	VerificationToken string     `json:"verification_token"`
	DNSTXTRecord      string     `json:"dns_txt_record"`
	VerifiedAt        *time.Time `json:"verified_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ── Service Account ───────────────────────────────────────────────────────────

// ServiceAccount is a machine identity within a tenant.
type ServiceAccount struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Role         string     `json:"role"`
	CustomRoleID string     `json:"custom_role_id,omitempty"`
	Enabled      bool       `json:"enabled"`
	CreatedBy    string     `json:"created_by"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ── API Key ───────────────────────────────────────────────────────────────────

// APIKeyOwnerType identifies whether an API key belongs to a user or service account.
type APIKeyOwnerType string

const (
	APIKeyOwnerUser           APIKeyOwnerType = "user"
	APIKeyOwnerServiceAccount APIKeyOwnerType = "service_account"
)

// APIKey represents a long-lived credential with scoped permissions.
// RawKey is only populated on creation or rotation and is never stored.
type APIKey struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Name       string          `json:"name"`
	KeyPrefix  string          `json:"key_prefix"`
	OwnerType  APIKeyOwnerType `json:"owner_type"`
	OwnerID    string          `json:"owner_id"`
	Scopes     []string        `json:"scopes"`
	LastUsedAt *time.Time      `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	Revoked    bool            `json:"revoked"`
	RevokedAt  *time.Time      `json:"revoked_at,omitempty"`
	RevokedBy  string          `json:"revoked_by,omitempty"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	// RawKey is set only on creation and rotation — never persisted.
	RawKey string `json:"raw_key,omitempty"`
}

// ── SCIM ──────────────────────────────────────────────────────────────────────

// SCIMUser is the SCIM 2.0 User resource representation.
type SCIMUser struct {
	Schemas    []string    `json:"schemas"`
	ID         string      `json:"id"`
	ExternalID string      `json:"externalId,omitempty"`
	UserName   string      `json:"userName"`
	Name       SCIMName    `json:"name"`
	Emails     []SCIMEmail `json:"emails"`
	Active     bool        `json:"active"`
	Groups     []SCIMRef   `json:"groups,omitempty"`
	Meta       SCIMMeta    `json:"meta"`
}

// SCIMName represents the structured name component of a SCIM User.
type SCIMName struct {
	Formatted  string `json:"formatted,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
}

// SCIMEmail represents an email address entry in a SCIM User.
type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// SCIMRef is a reference to another SCIM resource (used in groups/members).
type SCIMRef struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

// SCIMMeta contains SCIM resource metadata.
type SCIMMeta struct {
	ResourceType string    `json:"resourceType"`
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	Location     string    `json:"location,omitempty"`
}

// SCIMGroup is the SCIM 2.0 Group resource representation.
type SCIMGroup struct {
	Schemas     []string  `json:"schemas"`
	ID          string    `json:"id"`
	ExternalID  string    `json:"externalId,omitempty"`
	DisplayName string    `json:"displayName"`
	Members     []SCIMRef `json:"members,omitempty"`
	Meta        SCIMMeta  `json:"meta"`
}

// SCIMListResponse is the standard SCIM list response envelope.
type SCIMListResponse struct {
	Schemas      []string    `json:"schemas"`
	TotalResults int         `json:"totalResults"`
	StartIndex   int         `json:"startIndex"`
	ItemsPerPage int         `json:"itemsPerPage"`
	Resources    interface{} `json:"Resources"`
}

// SCIMPatchOp is the SCIM PATCH request body.
type SCIMPatchOp struct {
	Schemas    []string       `json:"schemas"`
	Operations []SCIMOperation `json:"Operations"`
}

// SCIMOperation is a single operation within a SCIM PATCH request.
type SCIMOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// SCIMError is the SCIM-compliant error response body.
type SCIMError struct {
	Schemas []string `json:"schemas"`
	Status  int      `json:"status"`
	Detail  string   `json:"detail"`
}
