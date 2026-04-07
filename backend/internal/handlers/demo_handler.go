package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type DemoHandler struct {
	demoService *services.DemoService
}

func NewDemoHandler(demoService *services.DemoService) *DemoHandler {
	return &DemoHandler{
		demoService: demoService,
	}
}

func (demoHandler *DemoHandler) RunScenario(responseWriter http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		api.WriteError(responseWriter, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	type demoRequest struct {
		Scenario string `json:"scenario"`
	}

	var req demoRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		api.WriteError(responseWriter, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Scenario = strings.TrimSpace(req.Scenario)
	if req.Scenario == "" {
		api.WriteError(responseWriter, http.StatusBadRequest, "scenario is required")
		return
	}

	if err := demoHandler.demoService.RunScenario(req.Scenario); err != nil {
		api.WriteError(responseWriter, http.StatusBadRequest, err.Error())
		return
	}

	api.WriteJSON(responseWriter, http.StatusOK, map[string]string{
		"status":   "ok",
		"scenario": req.Scenario,
	})
}
