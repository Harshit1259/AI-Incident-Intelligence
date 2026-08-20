package models

import "time"

// Auth models for Phase 1 JWT authentication.

// User represents a platform user with role-based access.
type User struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"` // never serialised to JSON
	Role          string    `json:"role"` // RoleAdmin | RoleOperator | RoleViewer (see models/rbac.go)
	SSOUserID     string    `json:"sso_user_id,omitempty"`
	SSOProviderID string    `json:"sso_provider_id,omitempty"`
	DisplayName   string    `json:"display_name,omitempty"`
	SCIMExternalID string   `json:"scim_external_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Tenant represents an isolated customer organisation.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginRequest is the JSON body for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest is the JSON body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	Role       string `json:"role"`       // RoleAdmin | RoleOperator | RoleViewer; only admin can assign admin
	TenantID   string `json:"tenant_id"`  // optional — defaults to "default"
	DataRegion string `json:"data_region"` // "us" | "eu" | "apac" — defaults to deployment region
}

// AuthResponse is returned after a successful login or register.
type AuthResponse struct {
	Token  string `json:"token"`
	User   User   `json:"user"`
}

// TokenClaims is what we encode into the JWT payload.
type TokenClaims struct {
	UserID        string   `json:"sub"`
	TenantID      string   `json:"tenant_id"`
	Role          string   `json:"role"`
	IsServiceAcct bool     `json:"is_sa,omitempty"`
	APIKeyID      string   `json:"key_id,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	Exp           int64    `json:"exp"`
	Iat           int64    `json:"iat"`
}

// CtxKey is used for storing auth claims in request context.
type CtxKey string

const ClaimsContextKey CtxKey = "auth_claims"
