import { apiRequest } from "./client.js";

export const getWizardConfig = () => apiRequest("/onboarding/wizard-config");
export const sendTestAlert = () => apiRequest("/onboarding/send-test", { method: "POST", body: "{}" });
export const advanceStep = (step) => apiRequest("/onboarding/step", { method: "POST", body: JSON.stringify({ step }) });
export const markMilestone = (milestone) => apiRequest("/onboarding/milestone", { method: "POST", body: JSON.stringify({ milestone }) });
export const getProgress = () => apiRequest("/onboarding/progress");
