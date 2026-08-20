package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// BillingService handles billing plan management and Stripe checkout.
type BillingService struct {
	store         *store.BillingStore
	stripeKey     string            // empty → dev mock mode
	priceIDs      map[string]string // plan tier → Stripe Price ID
	frontendOrigin string
}

// NewBillingService creates a BillingService.
// When stripeKey is non-empty, Checkout creates a real Stripe session.
// When empty (dev/test), Checkout falls back to a local mock subscription.
func NewBillingService(s *store.BillingStore, stripeKey string, priceIDs map[string]string, frontendOrigin string) *BillingService {
	return &BillingService{
		store:          s,
		stripeKey:      stripeKey,
		priceIDs:       priceIDs,
		frontendOrigin: frontendOrigin,
	}
}

// GetPlans returns all active billing plans.
func (s *BillingService) GetPlans() ([]models.BillingPlan, error) {
	return s.store.GetPlans()
}

// GetSubscription returns the current subscription for a tenant.
func (s *BillingService) GetSubscription(tenantID string) (*models.TenantSubscription, error) {
	return s.store.GetSubscription(tenantID)
}

// Checkout initiates a plan upgrade.
// When Stripe is configured (STRIPE_SECRET_KEY is set), it creates a real
// Stripe Checkout Session and returns the hosted payment URL — the frontend
// must redirect the user there. Status is "pending_payment" until Stripe
// confirms via webhook.
// Without Stripe (dev/test only), it immediately creates a local subscription.
func (s *BillingService) Checkout(tenantID, planID string) (*models.CheckoutResponse, error) {
	plan, err := s.store.GetPlanByID(planID)
	if err != nil {
		return nil, fmt.Errorf("lookup plan: %w", err)
	}
	if plan == nil {
		return nil, fmt.Errorf("plan %q not found", planID)
	}
	if !plan.Active {
		return nil, fmt.Errorf("plan %q is not active", planID)
	}

	if s.stripeKey != "" {
		return s.stripeCheckout(tenantID, plan)
	}
	return s.mockCheckout(tenantID, plan)
}

// stripeCheckout creates a Stripe Checkout Session and records a pending subscription.
func (s *BillingService) stripeCheckout(tenantID string, plan *models.BillingPlan) (*models.CheckoutResponse, error) {
	priceID, ok := s.priceIDs[plan.Tier]
	if !ok || priceID == "" {
		return nil, fmt.Errorf("no Stripe price configured for plan tier %q — set STRIPE_PRICE_%s", plan.Tier, strings.ToUpper(plan.Tier))
	}

	origin := s.frontendOrigin
	if origin == "" {
		origin = "http://localhost:5173"
	}
	successURL := origin + "/billing/success?session_id={CHECKOUT_SESSION_ID}"
	cancelURL := origin + "/billing/cancel"

	sessionURL, sessionID, err := s.createStripeSession(tenantID, priceID, successURL, cancelURL)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout session: %w", err)
	}

	// Record a pending subscription so we can match the Stripe webhook on completion.
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)
	sub := models.TenantSubscription{
		ID:                 sessionID,
		TenantID:           tenantID,
		PlanID:             plan.ID,
		Status:             "pending_payment",
		StripeCustomerID:   "",
		StripeSubID:        "",
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.store.UpsertSubscription(sub); err != nil {
		return nil, fmt.Errorf("save pending subscription: %w", err)
	}

	return &models.CheckoutResponse{
		SubscriptionID: sessionID,
		Status:         "pending_payment",
		Message:        fmt.Sprintf("Redirect to Stripe to complete payment for %s plan", plan.Name),
		CheckoutURL:    sessionURL,
	}, nil
}

// createStripeSession calls POST /v1/checkout/sessions on the Stripe API.
// No external SDK — raw HTTP with form-encoded body.
func (s *BillingService) createStripeSession(tenantID, priceID, successURL, cancelURL string) (sessionURL, sessionID string, err error) {
	body := url.Values{}
	body.Set("mode", "subscription")
	body.Set("line_items[0][price]", priceID)
	body.Set("line_items[0][quantity]", "1")
	body.Set("success_url", successURL)
	body.Set("cancel_url", cancelURL)
	body.Set("metadata[tenant_id]", tenantID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.stripe.com/v1/checkout/sessions",
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+s.stripeKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("stripe HTTP: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", "", fmt.Errorf("read stripe response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &apiErr)
		return "", "", fmt.Errorf("stripe API %d: %s", resp.StatusCode, apiErr.Error.Message)
	}

	var session struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &session); err != nil {
		return "", "", fmt.Errorf("decode stripe session: %w", err)
	}
	if session.URL == "" {
		return "", "", fmt.Errorf("stripe returned empty checkout URL")
	}
	return session.URL, session.ID, nil
}

// mockCheckout creates a local subscription immediately — dev/test only.
func (s *BillingService) mockCheckout(tenantID string, plan *models.BillingPlan) (*models.CheckoutResponse, error) {
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)
	subID := fmt.Sprintf("sub_%s_%d", tenantID, now.UnixNano())

	sub := models.TenantSubscription{
		ID:                 subID,
		TenantID:           tenantID,
		PlanID:             plan.ID,
		Status:             "active",
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.store.UpsertSubscription(sub); err != nil {
		return nil, fmt.Errorf("save subscription: %w", err)
	}
	return &models.CheckoutResponse{
		SubscriptionID: subID,
		Status:         "active",
		Message:        fmt.Sprintf("[DEV] Mock subscription created for %s plan — configure STRIPE_SECRET_KEY for real billing", plan.Name),
	}, nil
}

// CheckLimits returns usage summary and whether the tenant needs to upgrade.
func (s *BillingService) CheckLimits(tenantID string) (*models.UsageSummary, error) {
	return s.store.GetUsageSummary(tenantID)
}

// EnsureStarterPlan creates a starter plan subscription for a new tenant if none exists.
func (s *BillingService) EnsureStarterPlan(tenantID string) error {
	existing, err := s.store.GetSubscription(tenantID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	sub := models.TenantSubscription{
		ID:                 fmt.Sprintf("sub_%s_starter", tenantID),
		TenantID:           tenantID,
		PlanID:             "plan_starter",
		Status:             "active",
		CurrentPeriodStart: &now,
		CurrentPeriodEnd:   &periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return s.store.UpsertSubscription(sub)
}
