package handlers

import (
	"encoding/json"
	"net/http"
)

func HealthHandler(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(http.StatusOK)

	response := map[string]string{
		"status":  "ok",
		"service": "backend",
	}

	_ = json.NewEncoder(responseWriter).Encode(response)
}
