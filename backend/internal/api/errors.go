package api

// Machine-readable error codes — sent in every error response as "code".
// Clients parse Code; humans read Error. Never remove or rename a code once
// shipped — add new ones and deprecate old ones with a transitional period.
const (
	// Generic
	ErrCodeInternal           = "INTERNAL_ERROR"
	ErrCodeNotFound           = "NOT_FOUND"
	ErrCodeMethodNotAllowed   = "METHOD_NOT_ALLOWED"
	ErrCodeBadRequest         = "BAD_REQUEST"
	ErrCodeConflict           = "CONFLICT"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"

	// Auth
	ErrCodeTokenMissing  = "AUTH_TOKEN_MISSING"
	ErrCodeTokenInvalid  = "AUTH_TOKEN_INVALID"
	ErrCodeTokenExpired  = "AUTH_TOKEN_EXPIRED"
	ErrCodeForbidden     = "FORBIDDEN"

	// RBAC
	ErrCodeRoleInsufficient = "ROLE_INSUFFICIENT"

	// Tenant lifecycle
	ErrCodeTenantSuspended = "TENANT_SUSPENDED"
	ErrCodeTenantDisabled  = "TENANT_DISABLED"

	// Rate limiting
	ErrCodeRateLimitExceeded         = "RATE_LIMIT_EXCEEDED"
	ErrCodeMutationRateLimitExceeded = "MUTATION_RATE_LIMIT_EXCEEDED"

	// Billing
	ErrCodeBillingLimitReached = "BILLING_LIMIT_REACHED"
	ErrCodeBillingPlanNotFound = "BILLING_PLAN_NOT_FOUND"

	// Validation
	ErrCodeValidation = "VALIDATION_ERROR"
)

// statusToErrCode returns the default error code for an HTTP status.
// Use WriteErrorCode when you need a specific code.
func statusToErrCode(status int) string {
	switch status {
	case 400:
		return ErrCodeBadRequest
	case 401:
		return ErrCodeTokenInvalid
	case 403:
		return ErrCodeForbidden
	case 404:
		return ErrCodeNotFound
	case 405:
		return ErrCodeMethodNotAllowed
	case 409:
		return ErrCodeConflict
	case 429:
		return ErrCodeRateLimitExceeded
	case 503:
		return ErrCodeServiceUnavailable
	default:
		return ErrCodeInternal
	}
}
