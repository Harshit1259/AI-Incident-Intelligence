// BillingPanel.jsx — Phase 4, Week 10
// Stripe billing: Starter / Growth / Scale tiers. Upgrade prompts.

import { useState, useEffect, useCallback } from "react";
import { getBillingPlans, getSubscription, checkout, getUsage } from "../api/phase4.js";

const TIER_CONFIG = {
  starter: { color: "#6b7280", badge: "FREE", gradient: "rgba(107,114,128,0.1)" },
  growth:  { color: "#818cf8", badge: "POPULAR", gradient: "rgba(129,140,248,0.1)" },
  scale:   { color: "#10b981", badge: "ENTERPRISE", gradient: "rgba(16,185,129,0.1)" },
};

function PlanCard({ plan, currentPlanID, onSelect, loading }) {
  const cfg = TIER_CONFIG[plan.tier] || TIER_CONFIG.starter;
  const isCurrent = plan.id === currentPlanID;
  const price = plan.price_cents === 0 ? "Free" : `$${(plan.price_cents / 100).toFixed(0)}/mo`;
  const features = plan.features || [];
  const limits = plan.max_incidents === -1 ? "Unlimited" : `${plan.max_incidents} incidents`;

  return (
    <div className={`bill-plan ${isCurrent ? "bill-current" : ""}`} style={{ borderColor: cfg.color }}>
      {plan.tier === "growth" && <div className="bill-popular" style={{ background: cfg.color }}>MOST POPULAR</div>}
      <div className="bill-plan-header">
        <h3 style={{ margin: 0 }}>{plan.name}</h3>
        <div className="bill-price">{price}</div>
      </div>
      <div className="bill-limits">
        <div>{limits}</div>
        <div>{plan.max_users === -1 ? "Unlimited" : plan.max_users} users</div>
        <div>{plan.max_services === -1 ? "Unlimited" : plan.max_services} services</div>
      </div>
      <div className="bill-features">
        {features.map(f => (
          <div key={f} className="bill-feature">✓ {f.replace(/_/g, " ")}</div>
        ))}
      </div>
      {isCurrent ? (
        <div className="bill-current-badge" style={{ color: cfg.color }}>Current Plan</div>
      ) : (
        <button className="lux-primary-btn" onClick={() => onSelect(plan.id)} disabled={loading}
          style={{ background: cfg.color, borderColor: cfg.color }}>
          {loading ? "Processing..." : plan.price_cents === 0 ? "Downgrade" : "Upgrade"}
        </button>
      )}
    </div>
  );
}

function UsageBar({ label, used, max }) {
  const unlimited = max === -1;
  const pct = unlimited ? 10 : Math.min(100, (used / Math.max(1, max)) * 100);
  const color = pct >= 90 ? "#ef4444" : pct >= 70 ? "#f59e0b" : "#10b981";
  return (
    <div className="bill-usage-item">
      <div className="bill-usage-header">
        <span>{label}</span>
        <span>{used} / {unlimited ? "∞" : max}</span>
      </div>
      <div className="slo-budget-bar" style={{ height: 8 }}>
        <div className="slo-budget-fill" style={{ width: `${pct}%`, background: color, height: 8 }} />
      </div>
    </div>
  );
}

export default function BillingPanel() {
  const [plans, setPlans] = useState([]);
  const [sub, setSub] = useState(null);
  const [usage, setUsage] = useState(null);
  const [loading, setLoading] = useState(true);
  const [checkoutLoading, setCheckoutLoading] = useState(false);
  const [checkoutError, setCheckoutError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [p, s, u] = await Promise.all([
        getBillingPlans().catch(() => []),
        getSubscription().catch(() => null),
        getUsage().catch(() => null),
      ]);
      setPlans(Array.isArray(p) ? p : []);
      setSub(s);
      setUsage(u);
    } catch { /* ignore — individual sub-calls already have .catch(() => null) */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCheckout(planID) {
    setCheckoutLoading(true);
    setCheckoutError("");
    try {
      await checkout(planID);
      load();
    } catch (e) { setCheckoutError("Checkout failed: " + e.message); }
    finally { setCheckoutLoading(false); }
  }

  const currentPlanID = sub?.plan_id || "plan_starter";

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">BILLING & PLANS</div>
          <h2 style={{ margin: "0.25rem 0" }}>Choose Your Plan</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            The AIOps platform built for teams without a dedicated SRE.
          </div>
        </div>
      </div>

      {/* Usage Summary */}
      {usage && (
        <div className="bill-usage-section">
          <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>CURRENT USAGE</div>
          <div className="bill-usage-grid">
            <UsageBar label="Incidents" used={usage.incident_count || 0} max={usage.plan_limit?.max_incidents || 100} />
            <UsageBar label="Users" used={usage.user_count || 0} max={usage.plan_limit?.max_users || 3} />
            <UsageBar label="Services" used={usage.service_count || 0} max={usage.plan_limit?.max_services || 5} />
          </div>
          {usage.upgrade_needed && (
            <div className="bill-upgrade-banner">
              ⚠️ You're approaching your plan limits. Upgrade to keep creating incidents.
            </div>
          )}
        </div>
      )}

      {/* Subscription info */}
      {sub && (
        <div className="bill-sub-info">
          <span>Current: <strong>{sub.plan?.name || currentPlanID}</strong></span>
          <span className="lux-mini-chip" style={{ color: sub.status === "active" ? "#10b981" : "#f59e0b" }}>
            {(sub.status || "active").toUpperCase()}
          </span>
          {sub.current_period_end && (
            <span className="slo-service">Renews {new Date(sub.current_period_end).toLocaleDateString()}</span>
          )}
        </div>
      )}

      {checkoutError && (
        <div style={{ padding: "0.6rem 0.9rem", borderRadius: 8, marginBottom: "1rem", fontSize: "0.85rem", background: "rgba(127,29,29,0.25)", border: "1px solid rgba(248,113,113,0.3)", color: "#fca5a5" }}>
          {checkoutError}
        </div>
      )}

      {/* Plans */}
      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading plans...</div>
      ) : (
        <div className="bill-plans-grid">
          {plans.map(p => (
            <PlanCard key={p.id} plan={p} currentPlanID={currentPlanID} onSelect={handleCheckout} loading={checkoutLoading} />
          ))}
        </div>
      )}
    </div>
  );
}
