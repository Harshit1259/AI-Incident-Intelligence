import { useState } from "react";
import { AlertCircle, Eye, EyeOff, Lock, Mail, Zap, Shield, Bot, Brain } from "lucide-react";
import { login, register } from "../api/auth.js";

const FEATURES = [
  { icon: Brain,  text: "AI-powered root cause analysis" },
  { icon: Bot,    text: "Automated remediation agents" },
  { icon: Shield, text: "Enterprise policy & audit trail" },
  { icon: Zap,    text: "Real-time incident intelligence" },
];

export default function AuthScreen({ onAuthenticated }) {
  const [mode,     setMode]     = useState("login");
  const [email,    setEmail]    = useState("");
  const [password, setPassword] = useState("");
  const [showPass, setShowPass] = useState(false);
  const [error,    setError]    = useState("");
  const [loading,  setLoading]  = useState(false);

  async function handleSubmit(e) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const token = mode === "login"
        ? await login(email, password)
        : await register(email, password);
      if (!token) {
        setError(mode === "login"
          ? "Invalid email or password."
          : "Registration failed. Email may already be in use.");
      } else {
        onAuthenticated(token, email, mode === "register");
      }
    } catch (err) {
      setError(err.message || "Authentication failed.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="auth-overlay">
      {/* Grid background */}
      <div style={{
        position: "absolute", inset: 0, pointerEvents: "none",
        backgroundImage: `
          linear-gradient(rgba(0,102,255,0.04) 1px, transparent 1px),
          linear-gradient(90deg, rgba(0,102,255,0.04) 1px, transparent 1px)
        `,
        backgroundSize: "48px 48px",
      }} />

      <div style={{ display: "flex", gap: "clamp(32px, 6vw, 80px)", width: "100%", maxWidth: 960, zIndex: 1 }}>

        {/* Left — Brand */}
        <div style={{
          flex: 1, display: "none",
          flexDirection: "column", justifyContent: "center",
          padding: "0 40px 0 0",
        }}
          className="auth-brand-col"
        >
          <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 40 }}>
            <div style={{
              width: 44, height: 44, borderRadius: 12,
              background: "linear-gradient(135deg, var(--blue) 0%, var(--cyan) 100%)",
              display: "flex", alignItems: "center", justifyContent: "center",
              boxShadow: "0 4px 20px var(--blue-glow)",
            }}>
              <Zap size={22} color="#fff" />
            </div>
            <div>
              <div style={{ fontSize: 22, fontWeight: 800, letterSpacing: "-0.03em", color: "var(--t1)" }}>
                NeuroOps
              </div>
              <div style={{ fontSize: 11, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.10em", fontWeight: 600 }}>
                AI Incident Platform
              </div>
            </div>
          </div>

          <h1 style={{
            fontSize: "clamp(28px, 3.5vw, 40px)",
            fontWeight: 800,
            letterSpacing: "-0.04em",
            lineHeight: 1.15,
            color: "var(--t1)",
            marginBottom: 16,
          }}>
            Stop incidents.<br />
            Start intelligence.
          </h1>

          <p style={{ fontSize: 15, color: "var(--t2)", lineHeight: 1.7, maxWidth: 340, marginBottom: 40 }}>
            The AI-powered platform that detects, diagnoses, and resolves incidents
            before your customers notice.
          </p>

          <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            {FEATURES.map(({ icon: Icon, text }) => (
              <div key={text} style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <div style={{
                  width: 34, height: 34, borderRadius: 9,
                  background: "var(--surface-2)", border: "1px solid var(--border-hover)",
                  display: "flex", alignItems: "center", justifyContent: "center",
                  flexShrink: 0,
                }}>
                  <Icon size={16} color="var(--blue-lt)" />
                </div>
                <span style={{ fontSize: 14, color: "var(--t2)" }}>{text}</span>
              </div>
            ))}
          </div>
        </div>

        {/* Right — Form */}
        <div className="auth-card" style={{ flexShrink: 0 }}>
          {/* Logo (shown on all sizes) */}
          <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 32 }}>
            <div style={{
              width: 36, height: 36, borderRadius: 10,
              background: "linear-gradient(135deg, var(--blue) 0%, var(--cyan) 100%)",
              display: "flex", alignItems: "center", justifyContent: "center",
              boxShadow: "0 3px 14px var(--blue-glow)",
            }}>
              <Zap size={18} color="#fff" />
            </div>
            <div>
              <div style={{ fontSize: 16, fontWeight: 800, letterSpacing: "-0.02em", color: "var(--t1)" }}>
                NeuroOps
              </div>
              <div style={{ fontSize: 10, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.10em", fontWeight: 600 }}>
                AI Incident Platform
              </div>
            </div>
          </div>

          <div style={{ marginBottom: 28 }}>
            <h2 style={{ fontSize: 20, fontWeight: 700, letterSpacing: "-0.03em", color: "var(--t1)", marginBottom: 6 }}>
              {mode === "login" ? "Welcome back" : "Create account"}
            </h2>
            <p style={{ fontSize: 13, color: "var(--t3)", lineHeight: 1.6 }}>
              {mode === "login"
                ? "Sign in to your incident command center"
                : "Set up your NeuroOps workspace in seconds"}
            </p>
          </div>

          {error && (
            <div className="toast error" style={{ marginBottom: 20 }}>
              <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
            <div className="form-group">
              <label className="form-label">Email address</label>
              <div style={{ position: "relative" }}>
                <Mail size={13} style={{
                  position: "absolute", left: 12, top: "50%", transform: "translateY(-50%)",
                  color: "var(--t3)", pointerEvents: "none",
                }} />
                <input
                  type="email"
                  className="form-input"
                  value={email}
                  onChange={e => setEmail(e.target.value)}
                  placeholder="you@company.com"
                  required
                  style={{ paddingLeft: 36 }}
                />
              </div>
            </div>

            <div className="form-group">
              <label className="form-label">Password</label>
              <div style={{ position: "relative" }}>
                <Lock size={13} style={{
                  position: "absolute", left: 12, top: "50%", transform: "translateY(-50%)",
                  color: "var(--t3)", pointerEvents: "none",
                }} />
                <input
                  type={showPass ? "text" : "password"}
                  className="form-input"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  placeholder="••••••••"
                  required
                  style={{ paddingLeft: 36, paddingRight: 40 }}
                />
                <button
                  type="button"
                  onClick={() => setShowPass(v => !v)}
                  style={{
                    position: "absolute", right: 12, top: "50%", transform: "translateY(-50%)",
                    background: "none", border: "none", cursor: "pointer",
                    color: "var(--t3)", padding: 0, display: "flex", alignItems: "center",
                  }}
                >
                  {showPass ? <EyeOff size={13} /> : <Eye size={13} />}
                </button>
              </div>
            </div>

            <button
              type="submit"
              className="btn btn-primary"
              disabled={loading}
              style={{ width: "100%", justifyContent: "center", marginTop: 4, padding: "11px" }}
            >
              {loading ? (
                <>
                  <span className="spinner" style={{ width: 14, height: 14, borderWidth: 2 }} />
                  {mode === "login" ? "Signing in…" : "Creating account…"}
                </>
              ) : (
                mode === "login" ? "Sign In" : "Create Account"
              )}
            </button>
          </form>

          <div style={{ marginTop: 20, textAlign: "center", fontSize: 13, color: "var(--t3)" }}>
            {mode === "login" ? "No account?" : "Already have an account?"}{" "}
            <button
              onClick={() => { setMode(m => m === "login" ? "register" : "login"); setError(""); }}
              style={{
                background: "none", border: "none", cursor: "pointer",
                color: "var(--blue-lt)", fontWeight: 600, fontSize: 13,
              }}
            >
              {mode === "login" ? "Sign up free" : "Sign in"}
            </button>
          </div>

          <div style={{
            marginTop: 28, paddingTop: 20, borderTop: "1px solid var(--border)",
            display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10,
          }}>
            {["AI-powered RCA", "Auto-remediation", "Multi-tenant SaaS", "Predictive alerts"].map(f => (
              <div key={f} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, color: "var(--t3)" }}>
                <div style={{ width: 5, height: 5, borderRadius: "50%", background: "var(--blue)", flexShrink: 0 }} />
                {f}
              </div>
            ))}
          </div>
        </div>
      </div>

      <style>{`
        @media (min-width: 700px) {
          .auth-brand-col { display: flex !important; }
        }
      `}</style>
    </div>
  );
}
