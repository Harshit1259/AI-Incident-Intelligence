package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// BillingHandler exposes billing plan and subscription endpoints.
type BillingHandler struct {
	billingService *services.BillingService
}

func NewBillingHandler(bs *services.BillingService) *BillingHandler {
	return &BillingHandler{billingService: bs}
}

// HandlePlans handles GET /api/v1/billing/plans.
func (h *BillingHandler) HandlePlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	plans, err := h.billingService.GetPlans()
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to list plans")
		return
	}

	api.WriteJSON(w, http.StatusOK, plans)
}

// HandleSubscription handles GET /api/v1/billing/subscription.
func (h *BillingHandler) HandleSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	sub, err := h.billingService.GetSubscription(claims.TenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get subscription")
		return
	}

	if sub == nil {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}

	api.WriteJSON(w, http.StatusOK, sub)
}

// HandleCheckout handles POST /api/v1/billing/checkout.
func (h *BillingHandler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	var req models.CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PlanID == "" {
		api.WriteError(w, http.StatusBadRequest, "plan_id is required")
		return
	}

	resp, err := h.billingService.Checkout(claims.TenantID, req.PlanID)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, resp)
}

// HandleUsage handles GET /api/v1/billing/usage.
func (h *BillingHandler) HandleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "no auth claims")
		return
	}

	usage, err := h.billingService.CheckLimits(claims.TenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to get usage")
		return
	}

	api.WriteJSON(w, http.StatusOK, usage)
}
