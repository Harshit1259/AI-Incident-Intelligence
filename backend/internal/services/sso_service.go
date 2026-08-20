package services

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// ── JWKS cache ────────────────────────────────────────────────────────────────

// jwksCacheEntry holds a cached set of JWKs and the time they were fetched.
type jwksCacheEntry struct {
	keys      []jwk
	fetchedAt time.Time
}

var (
	jwksCache     sync.Map          // url → *jwksCacheEntry
	jwksCacheTTL  = 10 * time.Minute
)

// ── JWK types ─────────────────────────────────────────────────────────────────

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`   // RSA modulus
	E   string `json:"e"`   // RSA exponent
	Crv string `json:"crv"` // EC curve
	X   string `json:"x"`   // EC x
	Y   string `json:"y"`   // EC y
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

// ── OIDC discovery ─────────────────────────────────────────────────────────────

type oidcDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// ── SSOService ────────────────────────────────────────────────────────────────

// SSOService handles OIDC and SAML SSO flows.
type SSOService struct {
	ssoStore  *store.SSOStore
	userStore *store.UserStore
	client    *http.Client
}

// NewSSOService creates a new SSOService.
func NewSSOService(ssoStore *store.SSOStore, userStore *store.UserStore) *SSOService {
	return &SSOService{
		ssoStore:  ssoStore,
		userStore: userStore,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

// ── Provider CRUD ─────────────────────────────────────────────────────────────

// CreateProvider saves a new SSO provider config.
func (s *SSOService) CreateProvider(p models.SSOProvider) error {
	return s.ssoStore.CreateProvider(p)
}

// GetProvider returns an SSO provider by id.
func (s *SSOService) GetProvider(id, tenantID string) (*models.SSOProvider, error) {
	return s.ssoStore.GetProvider(id, tenantID)
}

// ListProviders returns all providers for a tenant.
func (s *SSOService) ListProviders(tenantID string) ([]models.SSOProvider, error) {
	return s.ssoStore.ListProviders(tenantID)
}

// UpdateProvider updates an existing SSO provider config.
func (s *SSOService) UpdateProvider(p models.SSOProvider) error {
	return s.ssoStore.UpdateProvider(p)
}

// DeleteProvider removes an SSO provider.
func (s *SSOService) DeleteProvider(id, tenantID string) error {
	return s.ssoStore.DeleteProvider(id, tenantID)
}

// ── OIDC Flow ─────────────────────────────────────────────────────────────────

// OIDCAuthURL is the result of initiating an OIDC login.
type OIDCAuthURL struct {
	AuthorizationURL string
	State            string
}

// InitiateOIDC generates a PKCE challenge, stores state, and returns the
// authorization URL to redirect the user to.
func (s *SSOService) InitiateOIDC(providerID, tenantID, redirectTo string) (*OIDCAuthURL, error) {
	provider, err := s.ssoStore.GetProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("sso: provider not found: %w", err)
	}
	if !provider.Enabled {
		return nil, errors.New("sso: provider is disabled")
	}

	discovery, err := s.fetchDiscovery(provider.DiscoveryURL)
	if err != nil {
		return nil, fmt.Errorf("sso: discovery fetch: %w", err)
	}

	// Generate PKCE code verifier and challenge.
	codeVerifier, codeChallenge, err := generatePKCE()
	if err != nil {
		return nil, fmt.Errorf("sso: pkce generate: %w", err)
	}

	// Generate opaque state token.
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("sso: state generate: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Persist state for callback verification.
	if err := s.ssoStore.SaveState(models.SSOState{
		State:        state,
		TenantID:     tenantID,
		ProviderID:   providerID,
		CodeVerifier: codeVerifier,
		RedirectTo:   redirectTo,
		ExpiresAt:    time.Now().Add(5 * time.Minute),
	}); err != nil {
		return nil, fmt.Errorf("sso: save state: %w", err)
	}

	scopes := provider.Scopes
	if scopes == "" {
		scopes = "openid email profile"
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", provider.ClientID)
	params.Set("redirect_uri", provider.ACSURL)
	params.Set("scope", scopes)
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	authURL := discovery.AuthorizationEndpoint + "?" + params.Encode()

	return &OIDCAuthURL{
		AuthorizationURL: authURL,
		State:            state,
	}, nil
}

// OIDCCallbackResult is the result of a successful OIDC callback.
type OIDCCallbackResult struct {
	User     models.User
	IsNew    bool
	RedirectTo string
}

// HandleOIDCCallback processes the OIDC callback, verifies the code and state,
// exchanges for tokens, verifies the ID token, and provisions the user.
func (s *SSOService) HandleOIDCCallback(stateToken, code string) (*OIDCCallbackResult, error) {
	// Retrieve and validate state.
	st, err := s.ssoStore.GetState(stateToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("sso: invalid or expired state")
		}
		return nil, fmt.Errorf("sso: get state: %w", err)
	}
	// Delete state immediately (one-time use).
	_ = s.ssoStore.DeleteState(stateToken)

	provider, err := s.ssoStore.GetProviderByID(st.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("sso: provider not found: %w", err)
	}

	discovery, err := s.fetchDiscovery(provider.DiscoveryURL)
	if err != nil {
		return nil, fmt.Errorf("sso: discovery fetch: %w", err)
	}

	// Exchange code for tokens.
	tokenResponse, err := s.exchangeCode(discovery.TokenEndpoint, provider, code, st.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("sso: token exchange: %w", err)
	}

	// Verify the ID token.
	claims, err := s.verifyIDToken(tokenResponse.IDToken, discovery.JWKSURI)
	if err != nil {
		return nil, fmt.Errorf("sso: id token verify: %w", err)
	}

	// Extract user attributes.
	email := extractStringClaim(claims, []string{"email"})
	if email == "" {
		return nil, errors.New("sso: no email claim in id token")
	}
	sub := extractStringClaim(claims, []string{"sub"})
	name := extractStringClaim(claims, []string{"name", "display_name"})
	if name == "" {
		givenName := extractStringClaim(claims, []string{"given_name"})
		familyName := extractStringClaim(claims, []string{"family_name"})
		name = strings.TrimSpace(givenName + " " + familyName)
	}

	// Apply attribute mapping overrides.
	if provider.AttributeMapping != nil {
		if mapped, ok := provider.AttributeMapping["email"]; ok {
			if v := extractStringClaim(claims, []string{mapped}); v != "" {
				email = v
			}
		}
	}

	user, isNew, err := s.GetOrCreateSSOUser(provider, sub, email, name)
	if err != nil {
		return nil, fmt.Errorf("sso: provision user: %w", err)
	}

	return &OIDCCallbackResult{
		User:       user,
		IsNew:      isNew,
		RedirectTo: st.RedirectTo,
	}, nil
}

// GetOrCreateSSOUser finds an existing user by SSO ID or email, or creates one
// if auto_provision is enabled on the provider.
func (s *SSOService) GetOrCreateSSOUser(
	provider *models.SSOProvider,
	ssoSubject, email, displayName string,
) (models.User, bool, error) {
	// Try to find by SSO user id first.
	existing, found := s.userStore.GetUserBySSOUserID(ssoSubject, provider.TenantID)
	if found {
		return existing, false, nil
	}
	// Fall back to email lookup.
	existing, found = s.userStore.GetUserByEmail(email)
	if found && existing.TenantID == provider.TenantID {
		// Update SSO link.
		_ = s.userStore.UpdateSSOFields(existing.ID, ssoSubject, provider.ID, displayName)
		existing.SSOUserID = ssoSubject
		return existing, false, nil
	}

	if !provider.AutoProvision {
		return models.User{}, false, errors.New("sso: user not found and auto-provision is disabled")
	}

	// Create the user.
	newUser := models.User{
		ID:           generateID(),
		TenantID:     provider.TenantID,
		Email:        email,
		PasswordHash: "", // no password for SSO users
		Role:         provider.DefaultRole,
		SSOUserID:    ssoSubject,
		SSOProviderID: provider.ID,
		DisplayName:  displayName,
	}
	if err := s.userStore.CreateUserSSO(newUser); err != nil {
		return models.User{}, false, fmt.Errorf("sso: create user: %w", err)
	}
	return newUser, true, nil
}

// ── SAML Flow ─────────────────────────────────────────────────────────────────

// SAMLAuthRequest is the HTTP-Redirect encoded AuthnRequest.
type SAMLAuthRequest struct {
	RedirectURL string
	RelayState  string
}

// InitiateSAML builds a SAML HTTP-Redirect AuthnRequest.
func (s *SSOService) InitiateSAML(providerID, tenantID, redirectTo string) (*SAMLAuthRequest, error) {
	provider, err := s.ssoStore.GetProviderByID(providerID)
	if err != nil {
		return nil, fmt.Errorf("saml: provider not found: %w", err)
	}
	if !provider.Enabled {
		return nil, errors.New("saml: provider is disabled")
	}
	if provider.IDPSSOUrl == "" {
		return nil, errors.New("saml: idp_sso_url not configured")
	}

	// Generate a request ID and relay state.
	relayStateBytes := make([]byte, 16)
	if _, err := rand.Read(relayStateBytes); err != nil {
		return nil, err
	}
	relayState := base64.RawURLEncoding.EncodeToString(relayStateBytes)
	requestID := "id_" + relayState

	// Build a minimal SAML AuthnRequest XML.
	acsURL := provider.ACSURL
	spEntityID := provider.SPEntityID
	issueInstant := time.Now().UTC().Format(time.RFC3339)

	authnRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<samlp:AuthnRequest xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"`+
		` xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"`+
		` ID="%s" Version="2.0" IssueInstant="%s"`+
		` AssertionConsumerServiceURL="%s" ProtocolBinding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST">`+
		`<saml:Issuer>%s</saml:Issuer>`+
		`</samlp:AuthnRequest>`,
		requestID, issueInstant, acsURL, spEntityID,
	)

	encoded := base64.StdEncoding.EncodeToString([]byte(authnRequest))
	params := url.Values{}
	params.Set("SAMLRequest", encoded)
	params.Set("RelayState", relayState)

	// Store relay state → redirectTo mapping using SSO state table.
	_ = s.ssoStore.SaveState(models.SSOState{
		State:      relayState,
		TenantID:   tenantID,
		ProviderID: providerID,
		RedirectTo: redirectTo,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	})

	redirectURL := provider.IDPSSOUrl + "?" + params.Encode()

	return &SAMLAuthRequest{
		RedirectURL: redirectURL,
		RelayState:  relayState,
	}, nil
}

// HandleSAMLCallback parses a Base64-decoded SAMLResponse assertion.
// Returns the provisioned user or an error.
func (s *SSOService) HandleSAMLCallback(samlResponse, relayState string) (*OIDCCallbackResult, error) {
	// Retrieve state for relay state.
	st, err := s.ssoStore.GetState(relayState)
	if err != nil {
		return nil, errors.New("saml: invalid or expired relay state")
	}
	_ = s.ssoStore.DeleteState(relayState)

	provider, err := s.ssoStore.GetProviderByID(st.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("saml: provider not found: %w", err)
	}

	// Decode the SAMLResponse (Base64).
	xmlBytes, err := base64.StdEncoding.DecodeString(samlResponse)
	if err != nil {
		return nil, fmt.Errorf("saml: decode response: %w", err)
	}

	// Extract NameID and attributes from the XML.
	// This is a minimal parser sufficient for the most common IdPs (Okta, Azure AD).
	nameID, email, displayName, err := parseSAMLAssertion(string(xmlBytes))
	if err != nil {
		return nil, fmt.Errorf("saml: parse assertion: %w", err)
	}
	if email == "" {
		email = nameID
	}

	user, isNew, err := s.GetOrCreateSSOUser(provider, nameID, email, displayName)
	if err != nil {
		return nil, err
	}

	return &OIDCCallbackResult{
		User:       user,
		IsNew:      isNew,
		RedirectTo: st.RedirectTo,
	}, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// fetchDiscovery fetches and parses an OIDC discovery document.
func (s *SSOService) fetchDiscovery(discoveryURL string) (*oidcDiscovery, error) {
	resp, err := s.client.Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("fetch discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch discovery: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var d oidcDiscovery
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("decode discovery: %w", err)
	}
	return &d, nil
}

type tokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// exchangeCode exchanges an authorization code for tokens.
func (s *SSOService) exchangeCode(
	tokenEndpoint string,
	provider *models.SSOProvider,
	code, codeVerifier string,
) (*tokenResponse, error) {
	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", provider.ACSURL)
	params.Set("client_id", provider.ClientID)
	params.Set("client_secret", provider.ClientSecret)
	params.Set("code_verifier", codeVerifier)

	resp, err := s.client.PostForm(tokenEndpoint, params)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange code: status %d: %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	return &tr, nil
}

// verifyIDToken fetches JWKS and verifies the ID token signature.
func (s *SSOService) verifyIDToken(tokenStr, jwksURI string) (map[string]interface{}, error) {
	keys, err := s.fetchJWKS(jwksURI)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}
	return verifyIDTokenWithKeys(tokenStr, keys)
}

// fetchJWKS fetches keys from a JWKS endpoint with a short-lived in-memory cache.
func (s *SSOService) fetchJWKS(uri string) ([]jwk, error) {
	if entry, ok := jwksCache.Load(uri); ok {
		e := entry.(*jwksCacheEntry)
		if time.Since(e.fetchedAt) < jwksCacheTTL {
			return e.keys, nil
		}
	}
	resp, err := s.client.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, err
	}
	jwksCache.Store(uri, &jwksCacheEntry{keys: jwks.Keys, fetchedAt: time.Now()})
	return jwks.Keys, nil
}

// verifyIDTokenWithKeys performs JWT signature verification for RS256 and ES256.
func verifyIDTokenWithKeys(tokenStr string, keys []jwk) (map[string]interface{}, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	// Find the matching key.
	var matchedKey *jwk
	for i := range keys {
		if keys[i].Kid == header.Kid || header.Kid == "" {
			matchedKey = &keys[i]
			break
		}
	}
	if matchedKey == nil {
		return nil, errors.New("no matching JWK for kid")
	}

	signingInput := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	switch header.Alg {
	case "RS256":
		pub, err := parseRSAPublicKey(matchedKey)
		if err != nil {
			return nil, fmt.Errorf("parse RSA key: %w", err)
		}
		hash := sha256.Sum256([]byte(signingInput))
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sigBytes); err != nil {
			return nil, fmt.Errorf("RSA signature invalid: %w", err)
		}

	case "ES256":
		pub, err := parseECPublicKey(matchedKey)
		if err != nil {
			return nil, fmt.Errorf("parse EC key: %w", err)
		}
		// ES256 signature is r || s each half the key length.
		half := len(sigBytes) / 2
		r := new(big.Int).SetBytes(sigBytes[:half])
		sv := new(big.Int).SetBytes(sigBytes[half:])
		hash := sha256.Sum256([]byte(signingInput))
		if !ecdsa.Verify(pub, hash[:], r, sv) {
			return nil, errors.New("ECDSA signature invalid")
		}

	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", header.Alg)
	}

	// Decode payload claims.
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	// Validate expiry.
	if exp, ok := claims["exp"]; ok {
		switch v := exp.(type) {
		case float64:
			if int64(v) < time.Now().Unix() {
				return nil, errors.New("id_token expired")
			}
		}
	}

	return claims, nil
}

// parseRSAPublicKey reconstructs an *rsa.PublicKey from a JWK.
func parseRSAPublicKey(k *jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode RSA modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode RSA exponent: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	eInt := int(new(big.Int).SetBytes(eBytes).Int64())
	return &rsa.PublicKey{N: n, E: eInt}, nil
}

// parseECPublicKey reconstructs an *ecdsa.PublicKey from a JWK.
func parseECPublicKey(k *jwk) (*ecdsa.PublicKey, error) {
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("decode EC X: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("decode EC Y: %w", err)
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	// Default to P-256 for ES256.
	curve := elliptic.P256()
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// generatePKCE produces a code_verifier and code_challenge (S256).
func generatePKCE() (verifier, challenge string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return
}

// extractStringClaim extracts the first non-empty string claim from a claims map.
func extractStringClaim(claims map[string]interface{}, keys []string) string {
	for _, k := range keys {
		if v, ok := claims[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// parseSAMLAssertion is a minimal SAML assertion parser that extracts
// NameID, email, and display name from unencrypted SAML XML.
// Sufficient for Okta, Azure AD, and Google Workspace.
func parseSAMLAssertion(xmlStr string) (nameID, email, displayName string, err error) {
	// Extract NameID.
	nameID = extractXMLText(xmlStr, "NameID")
	if nameID == "" {
		return "", "", "", errors.New("SAML: NameID not found in assertion")
	}

	// Extract AttributeValue elements.
	email = extractSAMLAttribute(xmlStr, []string{"email", "mail", "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress"})
	displayName = extractSAMLAttribute(xmlStr, []string{"displayName", "name", "http://schemas.microsoft.com/identity/claims/displayname"})

	return nameID, email, displayName, nil
}

// extractXMLText is a simple XML text content extractor for a named element.
func extractXMLText(xml, tag string) string {
	start := "<" + tag + ">"
	end := "</" + tag + ">"
	si := strings.Index(xml, start)
	if si == -1 {
		// Try with namespace prefix.
		si = strings.Index(xml, ":"+tag+">")
		if si == -1 {
			return ""
		}
		si += len(":" + tag + ">")
		ei := strings.Index(xml[si:], "</")
		if ei == -1 {
			return ""
		}
		return xml[si : si+ei]
	}
	si += len(start)
	ei := strings.Index(xml[si:], end)
	if ei == -1 {
		return ""
	}
	return xml[si : si+ei]
}

// extractSAMLAttribute searches for a SAML Attribute with one of the given names.
func extractSAMLAttribute(xml string, names []string) string {
	for _, name := range names {
		// Look for: Name="<name>" and then extract AttributeValue content.
		marker := `Name="` + name + `"`
		si := strings.Index(xml, marker)
		if si == -1 {
			continue
		}
		rest := xml[si:]
		val := extractXMLText(rest, "AttributeValue")
		if val != "" {
			return val
		}
	}
	return ""
}
