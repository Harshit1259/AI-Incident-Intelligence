package api

import (
	"encoding/json"
	"log"
	"net/http"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(responseWriter http.ResponseWriter, statusCode int, payload interface{}) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)

	if payload == nil {
		return
	}

	_ = json.NewEncoder(responseWriter).Encode(payload)
}

func WriteError(responseWriter http.ResponseWriter, statusCode int, message string) {
	WriteJSON(responseWriter, statusCode, ErrorResponse{Error: message})
}

// SafeHandler wraps a handler function that returns an error. If the handler
// returns a non-nil error, it logs the error and returns a 500 JSON response.
func SafeHandler(fn func(w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			log.Printf("handler error: %v", err)
			WriteError(w, http.StatusInternalServerError, "internal server error")
		}
	}
}
