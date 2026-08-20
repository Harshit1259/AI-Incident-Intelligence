// LandingPage.jsx — Phase 4, Week 11
// Landing + pricing page. "The AIOps platform for teams without a dedicated SRE."


const PLANS = [
  {
    tier: "starter", name: "Starter", price: "Free", priceSub: "forever",
    features: ["100 incidents/mo", "3 users", "5 services", "Correlation engine", "Alert dedup", "Basic RCA", "Status page"],
    cta: "Get Started Free", color: "#6b7280",
  },
  {
    tier: "growth", name: "Growth", price: "$500", priceSub: "/month", popular: true,
    features: ["1,000 incidents/mo", "10 users", "25 services", "AI RCA (Claude)", "Copilot assistant", "SLO tracking", "On-call scheduling", "All integrations", "Post-mortems"],
    cta: "Start Free Trial", color: "#818cf8",
  },
  {
    tier: "scale", name: "Scale", price: "$1,500", priceSub: "/month",
    features: ["Unlimited incidents", "Unlimited users", "Unlimited services", "Everything in Growth", "Anomaly detection", "ROI dashboard", "Weekly digest", "Priority support"],
    cta: "Contact Sales", color: "#10b981",
  },
];

const FEATURES = [
  { icon: "🔗", title: "100 Alerts → 1 Incident", desc: "Fingerprint dedup + time-window correlation collapses alert storms into actionable incidents." },
  { icon: "🤖", title: "AI Root Cause in Seconds", desc: "Claude analyzes events, changes, and failure signatures to explain what went wrong and why." },
  { icon: "📊", title: "SLO Tracking Built-In", desc: "Define SLOs, track error budgets, get breach predictions. No spreadsheets needed." },
  { icon: "📅", title: "On-Call Without PagerDuty", desc: "Weekly/daily rotations, follow-the-sun, overrides. Built for teams of 3-15." },
  { icon: "💬", title: "Slack War Room Auto-Created", desc: "P1 incidents auto-create a channel, post AI RCA, and let you ack/resolve from Slack." },
  { icon: "📋", title: "Post-Mortems in 5 Minutes", desc: "AI drafts the whole post-mortem. Edit, review, publish. Not 90 minutes — five." },
  { icon: "📈", title: "Anomaly Detection", desc: "\"DB connections at 87% and growing\" — alerts before users notice." },
  { icon: "💰", title: "ROI You Can Show Your CFO", desc: "Hours saved, cost avoided, MTTR reduction. Numbers that justify renewal." },
];

export default function LandingPage({ onGetStarted }) {
  return (
    <div className="lp-root">
      {/* Hero */}
      <header className="lp-hero">
        <div className="lp-hero-badge">AI-POWERED INCIDENT INTELLIGENCE</div>
        <h1 className="lp-hero-title">
          The AIOps platform built for teams<br />without a dedicated SRE.
        </h1>
        <p className="lp-hero-sub">
          Connect Datadog + GitHub in 30 min. Incidents flow automatically. Slack war room auto-creates.
          3-min incident resolution with AI root cause analysis.
        </p>
        <div className="lp-hero-actions">
          <button className="lux-primary-btn" onClick={onGetStarted} style={{ padding: "0.75rem 2rem", fontSize: "1rem" }}>
            Get Started Free
          </button>
          <a href="#pricing" className="lux-secondary-btn" style={{ padding: "0.75rem 2rem", fontSize: "1rem", textDecoration: "none" }}>
            View Pricing
          </a>
        </div>
        <div className="lp-hero-note">No credit card required · Free tier forever · 5-min setup</div>
      </header>

      {/* Features */}
      <section className="lp-features">
        <div className="lp-section-header">
          <div className="lux-eyebrow">CAPABILITIES</div>
          <h2>Everything you need to go from alert chaos to incident clarity.</h2>
        </div>
        <div className="lp-features-grid">
          {FEATURES.map(f => (
            <div key={f.title} className="lp-feature-card">
              <div className="lp-feature-icon">{f.icon}</div>
              <h3>{f.title}</h3>
              <p>{f.desc}</p>
            </div>
          ))}
        </div>
      </section>

      {/* Pricing */}
      <section className="lp-pricing" id="pricing">
        <div className="lp-section-header">
          <div className="lux-eyebrow">PRICING</div>
          <h2>Simple pricing. No per-seat surprises.</h2>
          <p className="lux-muted">Start free, upgrade when you're ready. All plans include core correlation and dedup.</p>
        </div>
        <div className="lp-plans-grid">
          {PLANS.map(p => (
            <div key={p.tier} className={`lp-plan ${p.popular ? "lp-plan-popular" : ""}`} style={{ borderColor: p.color }}>
              {p.popular && <div className="lp-plan-badge" style={{ background: p.color }}>MOST POPULAR</div>}
              <h3>{p.name}</h3>
              <div className="lp-plan-price">
                <span className="lp-plan-amount">{p.price}</span>
                <span className="lp-plan-period">{p.priceSub}</span>
              </div>
              <ul className="lp-plan-features">
                {p.features.map(f => <li key={f}>✓ {f}</li>)}
              </ul>
              <button className="lux-primary-btn" onClick={onGetStarted}
                style={{ background: p.color, borderColor: p.color, width: "100%" }}>
                {p.cta}
              </button>
            </div>
          ))}
        </div>
      </section>

      {/* CTA */}
      <section className="lp-cta">
        <h2>Ready to stop dashboard hunting?</h2>
        <p>Any engineer can sign up, connect their stack, and get their first AI RCA without talking to anyone.</p>
        <button className="lux-primary-btn" onClick={onGetStarted} style={{ padding: "0.75rem 2rem", fontSize: "1rem" }}>
          Get Your First AI RCA Free
        </button>
      </section>
    </div>
  );
}
