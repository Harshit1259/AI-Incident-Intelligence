package api

import (
	"encoding/json"
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
