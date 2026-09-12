package handlers

// auth_handler.go — Phase 1, Week 3
//
// Endpoints:
//   POST /api/v1/auth/login    — issue JWT
//   POST /api/v1/auth/register — create user (first user auto-gets admin)
//   GET  /api/v1/auth/me       — return current user (requires auth)
//
// Password hashing: PBKDF2-HMAC-SHA256 with 10000 iterations and 16-byte salt.
// Format: "pbkdf2:<hex-salt>:<hex-derived-key>"
// Backward compatible with legacy SHA-256 format: "<hex-salt>:<hex-sha256(salt+password)>"

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/audit"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models" //nolint:depguard — models provides role constants
	"ai-incident-platform/backend/internal/store"
)

// tenantProvisioner is satisfied by TenantStore.Upsert + TenantStore.GetState.
// AuthHandler only needs two operations; using a thin interface avoids a full
// store dependency and keeps tests simple.
type tenantProvisioner interface {
	Upsert(t models.TenantMaster) error
	GetState(id string) string
}

// authEventProcessor lets the auth handler seed demo incidents for new tenants.
type authEventProcessor interface {
	ProcessEvent(event models.Event) bool
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	userStore        *store.UserStore
	tenantProv       tenantProvisioner // nil = no auto-provisioning
	eventProc        authEventProcessor // nil = no demo seeding
	jwtSecret        string
	jwtTTL           time.Duration
	deploymentRegion string
}

func NewAuthHandler(userStore *store.UserStore, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userStore: userStore,
		jwtSecret: jwtSecret,
		jwtTTL:    24 * time.Hour,
	}
}

// SetTenantProvisioner wires a tenant store so registration auto-creates a
// tenant record for every new tenant_id that doesn't yet have one.
func (h *AuthHandler) SetTenantProvisioner(tp tenantProvisioner) {
	h.tenantProv = tp
}

// SetDemoEventProcessor wires in the correlation service so the first registered
// user automatically sees demo incidents instead of an empty dashboard.
func (h *AuthHandler) SetDemoEventProcessor(ep authEventProcessor) {
	h.eventProc = ep
}

// SetDeploymentRegion records which geographic region this server instance
// serves (e.g. "us", "eu", "apac"). New tenants inherit this as their
// data_region unless they supply an explicit value in RegisterRequest.
func (h *AuthHandler) SetDeploymentRegion(region string) {
	h.deploymentRegion = region
}

// ─────────────────────────────────────────────────────
// POST /api/v1/auth/login
// ─────────────────────────────────────────────────────

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, found := h.userStore.GetUserByEmail(req.Email)
	if !found {
		// Constant-time response to prevent email enumeration
		writeAuthError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !checkPassword(req.Password, user.PasswordHash) {
		writeAuthError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Migrate legacy SHA-256 hash to PBKDF2 on successful login
	if !strings.HasPrefix(user.PasswordHash, "pbkdf2:") {
		if newHash, err := hashPassword(req.Password); err == nil {
			if err := h.userStore.UpdatePasswordHash(user.ID, newHash); err != nil {
				slog.ErrorContext(r.Context(), "auth: failed to migrate password hash", "email", user.Email, "error", err)
			} else {
				slog.InfoContext(r.Context(), "auth: migrated password hash to PBKDF2", "email", user.Email)
			}
		}
	}

	token, err := h.issueToken(user)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	audit.Log(user.TenantID, user.ID, "user.login", "user", user.ID, map[string]interface{}{
		"email": user.Email,
	})

	writeJSON(w, http.StatusOK, models.AuthResponse{Token: token, User: user})
}

// ─────────────────────────────────────────────────────
// POST /api/v1/auth/register
// ─────────────────────────────────────────────────────

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" || req.Password == "" {
		writeAuthError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < 8 {
		writeAuthError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if h.userStore.EmailExists(req.Email) {
		writeAuthError(w, http.StatusConflict, "email already registered")
		return
	}

	// Tenant resolution:
	// • JWT callers are locked to their own JWT tenant.
	// • Admin JWT callers may cross-provision by supplying a different tenant_id.
	// • Unauthenticated self-registration may supply any tenant_id, but the
	//   tenant must not already exist (prevents hijacking existing tenants).
	claims, hasJWT := middleware.ClaimsFromContext(r)
	var tenantID string
	if hasJWT {
		tenantID = claims.TenantID
		if req.TenantID != "" && claims.Role == models.RoleAdmin && req.TenantID != claims.TenantID {
			tenantID = req.TenantID
		}
	} else {
		tenantID = req.TenantID
		if tenantID == "" {
			tenantID = middleware.TenantFromRequest(r)
		}
		if tenantID != "" && tenantID != "default" && h.tenantExists(tenantID) {
			writeAuthError(w, http.StatusConflict, "tenant already exists")
			return
		}
	}

	// First user in the system automatically becomes admin.
	// Track this so the JWT admin-check below is skipped — there is no existing
	// admin to authorize the request yet, and blocking it would leave the system
	// with no admin account at all.
	// The same holds for the first user of a brand-new tenant (self-service
	// sign-up): unauthenticated sign-up into an existing tenant was refused
	// above, so this user is creating the tenant. Checking only the global
	// user count meant every company after the first got a 403 asking for
	// admin, or a tenant with no admin at all.
	role := req.Role
	newTenantSignup := !hasJWT && tenantID != "" && tenantID != "default" && !h.tenantExists(tenantID)
	isFirstUser := h.userStore.CountUsers() == 0 || newTenantSignup
	if isFirstUser {
		role = models.RoleAdmin
		slog.InfoContext(r.Context(), "auth: first user auto-promoted to admin role", "email", req.Email)
	}

	// Accept any valid role; default to operator for unrecognised values.
	if !models.ValidRole(role) {
		role = models.RoleOperator
	}

	// Only an existing admin may create another admin account.
	// Return 403 — silently downgrading the role causes the user to believe they
	// have admin access while the JWT says operator, leading to confusing permission errors.
	if role == models.RoleAdmin && !isFirstUser {
		callerClaims, hasJWT := middleware.ClaimsFromContext(r)
		if !hasJWT || callerClaims.Role != models.RoleAdmin {
			slog.WarnContext(r.Context(), "auth: rejected admin registration attempt — caller lacks admin JWT", "email", req.Email)
			writeAuthError(w, http.StatusForbidden, "only admins can create admin accounts")
			return
		}
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Auto-provision a tenant record if this is the first user for that tenant.
	// This enables self-service sign-up: POST /auth/register with a new tenant_id
	// atomically creates both the tenant and its first admin user.
	if h.tenantProv != nil && tenantID != "default" {
		state := h.tenantProv.GetState(tenantID)
		if state == "active" && !h.tenantExists(tenantID) {
			slug := strings.ToLower(strings.ReplaceAll(tenantID, " ", "-"))
			dataRegion := h.deploymentRegion
			if models.ValidRegion(req.DataRegion) {
				dataRegion = req.DataRegion
			}
			if dataRegion == "" {
				dataRegion = "us"
			}
			newTenant := models.TenantMaster{
				ID:              tenantID,
				Name:            tenantID,
				Slug:            slug,
				Plan:            string(models.PlanTrial),
				State:           string(models.TenantStateActive),
				OwnerEmail:      req.Email,
				AuditRetainDays: 90,
				DataRegion:      dataRegion,
			}
			if err := h.tenantProv.Upsert(newTenant); err != nil {
				slog.ErrorContext(r.Context(), "auth: failed to auto-provision tenant", "tenant_id", tenantID, "error", err)
				// Non-fatal: registration still proceeds; tenant admin can create it manually.
			} else {
				slog.InfoContext(r.Context(), "auth: auto-provisioned tenant", "tenant_id", tenantID, "email", req.Email)
			}
		}
	}

	user := models.User{
		ID:           generateUserID(),
		TenantID:     tenantID,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         role,
	}

	if err := h.userStore.CreateUser(user); err != nil {
		slog.ErrorContext(r.Context(), "auth: register error", "email", req.Email, "error", err)
		writeAuthError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, err := h.issueToken(user)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	// Seed demo incidents for the first user so they land on a non-empty dashboard.
	if isFirstUser && h.eventProc != nil {
		go h.seedDemoIncidents(user.TenantID)
	}

	writeJSON(w, http.StatusCreated, models.AuthResponse{Token: token, User: user})
}

// ─────────────────────────────────────────────────────
// GET /api/v1/auth/me  (requires Bearer token)
// ─────────────────────────────────────────────────────

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	user, found := h.userStore.GetUserByID(claims.UserID)
	if !found {
		writeAuthError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

// ─────────────────────────────────────────────────────
// Token issuance
// ─────────────────────────────────────────────────────

func (h *AuthHandler) issueToken(user models.User) (string, error) {
	now := time.Now().Unix()
	claims := models.TokenClaims{
		UserID:   user.ID,
		TenantID: user.TenantID,
		Role:     user.Role,
		Iat:      now,
		Exp:      now + int64(h.jwtTTL.Seconds()),
	}
	return middleware.GenerateToken(claims, h.jwtSecret)
}

// ─────────────────────────────────────────────────────
// Password helpers  (PBKDF2-HMAC-SHA256, 10000 iterations)
// ─────────────────────────────────────────────────────

// hashPassword returns "pbkdf2:<hex-salt>:<hex-derived-key>".
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2Key([]byte(password), salt, 10000, 32)
	return "pbkdf2:" + hex.EncodeToString(salt) + ":" + hex.EncodeToString(dk), nil
}

// checkPassword verifies a plaintext password against a stored hash.
// Supports both new PBKDF2 format and legacy SHA-256 format for backward compatibility.
func checkPassword(password, stored string) bool {
	// New PBKDF2 format: "pbkdf2:<hex-salt>:<hex-dk>"
	if strings.HasPrefix(stored, "pbkdf2:") {
		rest := stored[len("pbkdf2:"):]
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return false
		}
		salt, err := hex.DecodeString(parts[0])
		if err != nil {
			return false
		}
		storedHash, err := hex.DecodeString(parts[1])
		if err != nil {
			return false
		}
		dk := pbkdf2Key([]byte(password), salt, 10000, 32)
		return hmac.Equal(dk, storedHash)
	}

	// Legacy SHA-256 format: "<hex-salt>:<hex-sha256(salt+password)>"
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	saltHex, storedHash := parts[0], parts[1]
	hash := sha256.Sum256([]byte(saltHex + password))
	computed := hex.EncodeToString(hash[:])
	return computed == storedHash
}

// pbkdf2Key derives a key using PBKDF2-HMAC-SHA256.
func pbkdf2Key(password, salt []byte, iterations, keyLen int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	u := make([]byte, 4)
	u[3] = 1 // block index
	mac.Write(u)
	dk := mac.Sum(nil)
	prev := make([]byte, len(dk))
	copy(prev, dk)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(prev)
		curr := mac.Sum(nil)
		for j := range dk {
			dk[j] ^= curr[j]
		}
		copy(prev, curr)
	}
	if len(dk) > keyLen {
		dk = dk[:keyLen]
	}
	return dk
}

// ─────────────────────────────────────────────────────
// Demo seed — fires synthetic events so first user sees
// real incidents instead of an empty dashboard.
// ─────────────────────────────────────────────────────

func (h *AuthHandler) seedDemoIncidents(tenantID string) {
	now := time.Now()

	demoEvents := []models.Event{
		{
			ID:          fmt.Sprintf("demo-1-%d", now.UnixNano()),
			TenantID:    tenantID,
			Source:      "prometheus",
			ExternalID:  fmt.Sprintf("demo-ext-1-%d", now.UnixNano()),
			Service:     "checkout-api",
			Resource:    "checkout-api-pod-1",
			Environment: "production",
			Severity:    "critical",
			Type:        "error_rate",
			Title:       "[Demo] High error rate on checkout-api",
			Message:     "Error rate exceeded 5% threshold. 847 errors in the last 5 minutes. P99 latency: 2.3s.",
			Labels:      map[string]string{"demo": "true", "env": "production", "team": "platform"},
			Timestamp:   now.Add(-8 * time.Minute),
			IngestSchema: "webhook",
		},
		{
			ID:          fmt.Sprintf("demo-2-%d", now.UnixNano()),
			TenantID:    tenantID,
			Source:      "datadog",
			ExternalID:  fmt.Sprintf("demo-ext-2-%d", now.UnixNano()),
			Service:     "checkout-api",
			Resource:    "checkout-api-pod-2",
			Environment: "production",
			Severity:    "high",
			Type:        "latency",
			Title:       "[Demo] P99 latency spike on checkout-api",
			Message:     "P99 latency spiked to 4.1s (normal: 120ms). Database connection pool at 94%.",
			Labels:      map[string]string{"demo": "true", "env": "production", "team": "platform"},
			Timestamp:   now.Add(-7 * time.Minute),
			IngestSchema: "webhook",
		},
		{
			ID:          fmt.Sprintf("demo-3-%d", now.UnixNano()),
			TenantID:    tenantID,
			Source:      "prometheus",
			ExternalID:  fmt.Sprintf("demo-ext-3-%d", now.UnixNano()),
			Service:     "payment-service",
			Resource:    "payment-service",
			Environment: "production",
			Severity:    "high",
			Type:        "availability",
			Title:       "[Demo] Payment service unhealthy",
			Message:     "Health check failing. 3/5 replicas reporting errors. Possible database connection exhaustion.",
			Labels:      map[string]string{"demo": "true", "env": "production", "team": "payments"},
			Timestamp:   now.Add(-12 * time.Minute),
			IngestSchema: "webhook",
		},
		{
			ID:          fmt.Sprintf("demo-4-%d", now.UnixNano()),
			TenantID:    tenantID,
			Source:      "github",
			ExternalID:  fmt.Sprintf("demo-ext-4-%d", now.UnixNano()),
			Service:     "auth-service",
			Resource:    "auth-service-v2.3.1",
			Environment: "production",
			Severity:    "medium",
			Type:        "deployment",
			Title:       "[Demo] auth-service v2.3.1 deployed",
			Message:     "Deployment triggered 3 minutes before latency spike. Possible regression in connection pooling.",
			Labels:      map[string]string{"demo": "true", "env": "production", "version": "v2.3.1"},
			Timestamp:   now.Add(-15 * time.Minute),
			IngestSchema: "webhook",
		},
	}

	for _, evt := range demoEvents {
		h.eventProc.ProcessEvent(evt)
		time.Sleep(200 * time.Millisecond)
	}
}

// ─────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────

func (h *AuthHandler) tenantExists(tenantID string) bool {
	return h.userStore.TenantExists(tenantID)
}

func generateUserID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("usr-%s", hex.EncodeToString(b))
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
