package middleware

// jwt_auth.go — Phase 1, Week 3
//
// Pure-stdlib JWT implementation using HS256 (HMAC-SHA256).
// No external dependencies.
//
// Token format: base64url(header).base64url(payload).base64url(signature)
// where signature = HMAC-SHA256(header + "." + payload, secret)

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
)

// ─────────────────────────────────────────────────────
// Token generation
// ─────────────────────────────────────────────────────

// GenerateToken creates a signed HS256 JWT for the given claims.
func GenerateToken(claims models.TokenClaims, secret string) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	header := base64URLEncode(headerJSON)
	payload := base64URLEncode(payloadJSON)
	signingInput := header + "." + payload

	sig := computeHMAC(signingInput, secret)
	return signingInput + "." + base64URLEncode(sig), nil
}

// ─────────────────────────────────────────────────────
// Token verification
// ─────────────────────────────────────────────────────

// VerifyToken parses and validates a JWT string.
// Returns the claims on success, or an error if the token is invalid/expired.
func VerifyToken(tokenStr, secret string) (models.TokenClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return models.TokenClaims{}, fmt.Errorf("jwt: malformed token")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := base64URLEncode(computeHMAC(signingInput, secret))

	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return models.TokenClaims{}, fmt.Errorf("jwt: invalid signature")
	}

	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return models.TokenClaims{}, fmt.Errorf("jwt: decode payload: %w", err)
	}

	var claims models.TokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return models.TokenClaims{}, fmt.Errorf("jwt: unmarshal claims: %w", err)
	}

	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return models.TokenClaims{}, fmt.Errorf("jwt: token expired")
	}

	return claims, nil
}

// ─────────────────────────────────────────────────────
// HTTP middleware
// ─────────────────────────────────────────────────────

// RequireAuth returns middleware that enforces a valid JWT.
// The parsed claims are stored in the request context under models.ClaimsContextKey.
func RequireAuth(secret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenStr, err := extractBearerToken(r)
		if err != nil {
			api.WriteErrorCode(w, http.StatusUnauthorized,
				"missing or malformed authorization header", api.ErrCodeTokenMissing)
			return
		}

		claims, err := VerifyToken(tokenStr, secret)
		if err != nil {
			code := api.ErrCodeTokenInvalid
			if strings.Contains(err.Error(), "expired") {
				code = api.ErrCodeTokenExpired
			}
			api.WriteErrorCode(w, http.StatusUnauthorized, err.Error(), code)
			return
		}

		ctx := context.WithValue(r.Context(), models.ClaimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

// RequireRole wraps RequireAuth and additionally enforces that the caller has
// at least the specified role. Role hierarchy: admin > operator.
func RequireRole(secret, role string, next http.HandlerFunc) http.HandlerFunc {
	return RequireAuth(secret, func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(models.ClaimsContextKey).(models.TokenClaims)
		if !ok {
			api.WriteErrorCode(w, http.StatusUnauthorized,
				"no auth claims in context", api.ErrCodeTokenInvalid)
			return
		}
		if !hasRole(claims.Role, role) {
			api.WriteErrorCode(w, http.StatusForbidden,
				"insufficient permissions", api.ErrCodeRoleInsufficient)
			return
		}
		next(w, r)
	})
}

// ClaimsFromContext retrieves the token claims stored by RequireAuth middleware.
func ClaimsFromContext(r *http.Request) (models.TokenClaims, bool) {
	claims, ok := r.Context().Value(models.ClaimsContextKey).(models.TokenClaims)
	return claims, ok
}

// ─────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────

func extractBearerToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", fmt.Errorf("no Authorization header")
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("Authorization header must start with 'Bearer '")
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	if token == "" {
		return "", fmt.Errorf("empty bearer token")
	}
	return token, nil
}

func computeHMAC(data, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// hasRole returns true if userRole satisfies the required minimum role.
// Delegates to the models.RoleSatisfies hierarchy: admin > operator > viewer.
func hasRole(userRole, required string) bool {
	return models.RoleSatisfies(userRole, required)
}
