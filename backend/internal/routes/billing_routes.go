package routes

import (
	"net/http"

	"ai-incident-platform/backend/internal/handlers"
)

func registerBillingRoutes(
	mux *http.ServeMux,
	withAuth func(http.HandlerFunc) http.Handler,
	requireOperator func(http.HandlerFunc) http.HandlerFunc,
	billingHandler *handlers.BillingHandler,
	onboardingHandler *handlers.OnboardingHandler,
) {
	// Billing — financial data; viewers are excluded.
	mux.Handle("/api/v1/billing/plans", withAuth(requireOperator(billingHandler.HandlePlans)))
	mux.Handle("/api/v1/billing/subscription", withAuth(requireOperator(billingHandler.HandleSubscription)))
	mux.Handle("/api/v1/billing/checkout", withAuth(requireOperator(billingHandler.HandleCheckout)))
	mux.Handle("/api/v1/billing/usage", withAuth(requireOperator(billingHandler.HandleUsage)))

	// Onboarding — operator+ (setup actions)
	mux.Handle("/api/v1/onboarding/progress", withAuth(requireOperator(onboardingHandler.HandleProgress)))
	mux.Handle("/api/v1/onboarding/step", withAuth(requireOperator(onboardingHandler.HandleStep)))
	mux.Handle("/api/v1/onboarding/milestone", withAuth(requireOperator(onboardingHandler.HandleMilestone)))
	// Wizard config (URLs + progress in one call) and server-side test alert injection
	mux.Handle("/api/v1/onboarding/wizard-config", withAuth(requireOperator(onboardingHandler.HandleWizardConfig)))
	mux.Handle("/api/v1/onboarding/send-test", withAuth(requireOperator(onboardingHandler.HandleSendTest)))
}
