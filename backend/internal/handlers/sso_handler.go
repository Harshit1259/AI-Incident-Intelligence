package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// SSOHandler handles SSO provider management and the OIDC/SAML login flows.
type SSOHandler struct {
	ssoSvc    *services.SSOService
	jwtSecret string
}

// NewSSOHandler creates a new SSOHandler.
func NewSSOHandler(ssoSvc *services.SSOService, jwtSecret string) *SSOHandler {
	return &SSOHandler{ssoSvc: ssoSvc, jwtSecret: jwtSecret}
}

// Handle dispatches SSO routes.
func (h *SSOHandler) Handle(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	// ── Public login/callback flows ────────────────────────────────────────────
	case strings.HasPrefix(path, "/api/v1/sso/login/"):
		h.handleLogin(w, r)

	case path == "/api/v1/sso/callback":
		h.handleOIDCCallback(w, r)

	case path == "/api/v1/sso/saml/callback":
		h.handleSAMLCallback(w, r)

	// ── Provider management ────────────────────────────────────────────────────
	case path == "/api/v1/sso/providers":
		switch r.Method {
		case http.MethodGet:
			h.listProviders(w, r)
		case http.MethodPost:
			h.createProvider(w, r)
		default:
			ssoWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	case strings.HasPrefix(path, "/api/v1/sso/providers/"):
		id := strings.TrimPrefix(path, "/api/v1/sso/providers/")
		switch r.Method {
		case http.MethodGet:
			h.getProvider(w, r, id)
		case http.MethodPut:
			h.updateProvider(w, r, id)
		case http.MethodDelete:
			h.deleteProvider(w, r, id)
		default:
			ssoWriteErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}

	default:
		ssoWriteErr(w, http.StatusNotFound, "sso: route not found")
	}
}

// handleLogin redirects the user to the IdP authorization endpoint.
// GET /api/v1/sso/login/:provider_id?redirect_to=...&tenant_id=...
func (h *SSOHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	providerID := strings.TrimPrefix(r.URL.Path, "/api/v1/sso/login/")
	if providerID == "" {
		ssoWriteErr(w, http.StatusBadRequest, "sso: provider_id required")
		return
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}
	redirectTo := r.URL.Query().Get("redirect_to")

	provider, err := h.ssoSvc.GetProvider(providerID, tenantID)
	if err != nil {
		ssoWriteErr(w, http.StatusNotFound, "sso: provider not found")
		return
	}

	switch provider.ProviderType {
	case models.SSOTypeOIDC:
		result, err := h.ssoSvc.InitiateOIDC(providerID, tenantID, redirectTo)
		if err != nil {
			ssoWriteErr(w, http.StatusInternalServerError, "sso: "+err.Error())
			return
		}
		http.Redirect(w, r, result.AuthorizationURL, http.StatusFound)

	case models.SSOTypeSAML:
		result, err := h.ssoSvc.InitiateSAML(providerID, tenantID, redirectTo)
		if err != nil {
			ssoWriteErr(w, http.StatusInternalServerError, "sso: "+err.Error())
			return
		}
		http.Redirect(w, r, result.RedirectURL, http.StatusFound)

	default:
		ssoWriteErr(w, http.StatusBadRequest, "sso: unsupported provider type")
	}
}

// handleOIDCCallback processes the OIDC authorization code callback.
// GET /api/v1/sso/callback?code=...&state=...
func (h *SSOHandler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		ssoWriteErr(w, http.StatusBadRequest, "sso: code and state required")
		return
	}

	result, err := h.ssoSvc.HandleOIDCCallback(state, code)
	if err != nil {
		ssoWriteErr(w, http.StatusUnauthorized, "sso: "+err.Error())
		return
	}

	token, err := h.issueJWT(result.User)
	if err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, "sso: jwt issue: "+err.Error())
		return
	}

	if result.RedirectTo != "" {
		redirect := result.RedirectTo
		if strings.Contains(redirect, "?") {
			redirect += "&token=" + token
		} else {
			redirect += "?token=" + token
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	ssoWriteJSON(w, http.StatusOK, map[string]interface{}{
		"token":  token,
		"user":   result.User,
		"is_new": result.IsNew,
	})
}

// handleSAMLCallback processes the SAML POST callback.
// POST /api/v1/sso/saml/callback
func (h *SSOHandler) handleSAMLCallback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		ssoWriteErr(w, http.StatusBadRequest, "sso: parse form: "+err.Error())
		return
	}
	samlResponse := r.FormValue("SAMLResponse")
	relayState := r.FormValue("RelayState")

	if samlResponse == "" {
		ssoWriteErr(w, http.StatusBadRequest, "sso: SAMLResponse required")
		return
	}

	result, err := h.ssoSvc.HandleSAMLCallback(samlResponse, relayState)
	if err != nil {
		ssoWriteErr(w, http.StatusUnauthorized, "sso: "+err.Error())
		return
	}

	token, err := h.issueJWT(result.User)
	if err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, "sso: jwt issue: "+err.Error())
		return
	}

	if result.RedirectTo != "" {
		redirect := result.RedirectTo
		if strings.Contains(redirect, "?") {
			redirect += "&token=" + token
		} else {
			redirect += "?token=" + token
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	ssoWriteJSON(w, http.StatusOK, map[string]interface{}{
		"token":  token,
		"user":   result.User,
		"is_new": result.IsNew,
	})
}

// ── Provider management ───────────────────────────────────────────────────────

func (h *SSOHandler) listProviders(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		ssoWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	providers, err := h.ssoSvc.ListProviders(claims.TenantID)
	if err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if providers == nil {
		providers = []models.SSOProvider{}
	}
	ssoWriteJSON(w, http.StatusOK, map[string]interface{}{"providers": providers})
}

func (h *SSOHandler) createProvider(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		ssoWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var p models.SSOProvider
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		ssoWriteErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	var err error
	p.ID, err = ssoNewID()
	if err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, "failed to generate provider ID")
		return
	}
	p.TenantID = claims.TenantID
	if p.ProviderType == "" {
		p.ProviderType = models.SSOTypeOIDC
	}
	if p.DefaultRole == "" {
		p.DefaultRole = models.RoleViewer
	}
	p.Enabled = true

	if err := h.ssoSvc.CreateProvider(p); err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Never return secret material in the response.
	p.ClientSecret = ""
	p.IDPCert = ""
	ssoWriteJSON(w, http.StatusCreated, p)
}

func (h *SSOHandler) getProvider(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		ssoWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	p, err := h.ssoSvc.GetProvider(id, claims.TenantID)
	if err != nil {
		ssoWriteErr(w, http.StatusNotFound, "provider not found")
		return
	}
	p.ClientSecret = ""
	p.IDPCert = ""
	ssoWriteJSON(w, http.StatusOK, p)
}

func (h *SSOHandler) updateProvider(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		ssoWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	existing, err := h.ssoSvc.GetProvider(id, claims.TenantID)
	if err != nil {
		ssoWriteErr(w, http.StatusNotFound, "provider not found")
		return
	}

	var update models.SSOProvider
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		ssoWriteErr(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	update.ID = existing.ID
	update.TenantID = existing.TenantID
	if update.ClientSecret == "" {
		update.ClientSecret = existing.ClientSecret
	}
	if update.IDPCert == "" {
		update.IDPCert = existing.IDPCert
	}

	if err := h.ssoSvc.UpdateProvider(update); err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	update.ClientSecret = ""
	update.IDPCert = ""
	ssoWriteJSON(w, http.StatusOK, update)
}

func (h *SSOHandler) deleteProvider(w http.ResponseWriter, r *http.Request, id string) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		ssoWriteErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.ssoSvc.DeleteProvider(id, claims.TenantID); err != nil {
		ssoWriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── internal helpers ──────────────────────────────────────────────────────────

func (h *SSOHandler) issueJWT(user models.User) (string, error) {
	now := time.Now().Unix()
	claims := models.TokenClaims{
		UserID:   user.ID,
		TenantID: user.TenantID,
		Role:     user.Role,
		Exp:      now + 86400,
		Iat:      now,
	}
	return middleware.GenerateToken(claims, h.jwtSecret)
}

func ssoNewID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand unavailable: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ssoWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ssoWriteErr(w http.ResponseWriter, status int, msg string) {
	ssoWriteJSON(w, status, map[string]string{"error": msg})
}
