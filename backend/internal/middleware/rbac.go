package middleware

import (
	"fmt"
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
)

// RequireMinRole returns middleware that enforces a minimum role.
// It relies on RequireAuth having already placed claims in the context.
//
// Role hierarchy (models/rbac.go): admin(3) > operator(2) > viewer(1)
//
// Use the role constants, never bare strings:
//   middleware.RequireMinRole(models.RoleAdmin)
//   middleware.RequireMinRole(models.RoleOperator)
//   middleware.RequireMinRole(models.RoleViewer)  ← effectively just requires auth
func RequireMinRole(minRole string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(models.ClaimsContextKey).(models.TokenClaims)
			if !ok {
				writeRBACError(w, http.StatusUnauthorized, "no auth claims in context")
				return
			}
			if !models.RoleSatisfies(claims.Role, minRole) {
				writeRBACError(w, http.StatusForbidden,
					fmt.Sprintf("role '%s' does not have permission; requires '%s' or higher", claims.Role, minRole))
				return
			}
			next(w, r)
		}
	}
}

// RequireRoleMiddleware is retained for backward compatibility.
// New code should use RequireMinRole with a role constant.
func RequireRoleMiddleware(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(models.ClaimsContextKey).(models.TokenClaims)
			if !ok {
				writeRBACError(w, http.StatusUnauthorized, "no auth claims in context")
				return
			}
			// Admin always passes.
			if claims.Role == models.RoleAdmin {
				next(w, r)
				return
			}
			for _, role := range roles {
				if claims.Role == role {
					next(w, r)
					return
				}
			}
			writeRBACError(w, http.StatusForbidden, "insufficient permissions")
		}
	}
}

func writeRBACError(w http.ResponseWriter, status int, msg string) {
	code := api.ErrCodeForbidden
	if status == http.StatusUnauthorized {
		code = api.ErrCodeTokenInvalid
	} else if status == http.StatusForbidden {
		code = api.ErrCodeRoleInsufficient
	}
	api.WriteErrorCode(w, status, msg, code)
}

