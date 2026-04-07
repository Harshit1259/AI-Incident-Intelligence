package handlers

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/store"
)

type DevHandler struct {
	devStore *store.DevStore
}

func NewDevHandler(devStore *store.DevStore) *DevHandler {
	return &DevHandler{
		devStore: devStore,
	}
}

func (devHandler *DevHandler) Reset(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.WriteError(responseWriter, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := devHandler.devStore.ResetAll(); err != nil {
		api.WriteError(responseWriter, http.StatusInternalServerError, "failed to reset data")
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status": "reset complete",
	})
}
