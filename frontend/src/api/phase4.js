// phase4.js — Phase 4 API client (Billing, Onboarding)

import { apiRequest } from "./client.js";

// ── Billing ──
export const getBillingPlans = () => apiRequest("/billing/plans");
export const getSubscription = () => apiRequest("/billing/subscription");
export const checkout = (planID) => apiRequest("/billing/checkout", { method: "POST", body: JSON.stringify({ plan_id: planID }) });
export const getUsage = () => apiRequest("/billing/usage");

// ── Onboarding ──
export const getOnboardingProgress = () => apiRequest("/onboarding/progress");
export const advanceOnboardingStep = (step) => apiRequest("/onboarding/step", { method: "POST", body: JSON.stringify({ step }) });
export const markMilestone = (milestone) => apiRequest("/onboarding/milestone", { method: "POST", body: JSON.stringify({ milestone }) });
