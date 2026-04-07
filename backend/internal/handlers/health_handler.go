package handlers

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
)

func HealthHandler(responseWriter http.ResponseWriter, request *http.Request) {
	api.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "backend",
	})
}
