package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorResponse is the canonical JSON error envelope returned by every handler.
// Clients should branch on Code (stable); Error is for humans only.
type ErrorResponse struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

// WriteJSON serialises payload as JSON and writes it with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError writes a JSON error response. The machine-readable Code is derived
// from statusCode via statusToErrCode. Use WriteErrorCode for a specific code.
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	WriteErrorCode(w, statusCode, message, statusToErrCode(statusCode))
}

// WriteErrorCode writes a JSON error response with an explicit machine-readable
// code. The request ID is automatically included when the RequestID middleware
// has already set the X-Request-ID response header.
func WriteErrorCode(w http.ResponseWriter, statusCode int, message, code string) {
	requestID := w.Header().Get("X-Request-ID")
	WriteJSON(w, statusCode, ErrorResponse{
		Error:     message,
		Code:      code,
		RequestID: requestID,
	})
}

// SafeHandler wraps a handler function that returns an error. If the handler
// returns a non-nil error, it logs it and returns a structured 500 response.
func SafeHandler(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			slog.ErrorContext(r.Context(), "handler error", "error", err)
			WriteErrorCode(w, http.StatusInternalServerError, "internal server error", ErrCodeInternal)
		}
	}
}
