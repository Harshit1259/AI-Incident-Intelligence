import React from "react";
import AppShell from "./components/AppShell";
import { PublicStatusPage } from "./components/StatusPageView";
import "./design-system.css";

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }
  componentDidCatch(error, info) {
    console.error("[ErrorBoundary]", error, info?.componentStack);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div style={{
          display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center",
          minHeight: "100vh", padding: "2rem",
          background: "var(--bg-base, #070B13)", color: "var(--text-primary, #F1F5F9)", textAlign: "center",
          fontFamily: "Inter, system-ui, sans-serif",
        }}>
          <div style={{
            width: 64, height: 64, borderRadius: 20, marginBottom: "1.5rem",
            background: "rgba(239,68,68,0.1)", border: "1px solid rgba(239,68,68,0.3)",
            display: "flex", alignItems: "center", justifyContent: "center", fontSize: 28,
          }}>⚠</div>
          <h2 style={{ margin: "0 0 0.5rem", fontSize: 20, fontWeight: 700 }}>Something went wrong</h2>
          <p style={{ color: "#94A3B8", maxWidth: 440, lineHeight: 1.6, fontSize: "0.875rem", marginBottom: "1.5rem" }}>
            {this.state.error?.message || "An unexpected error occurred."}
          </p>
          <button
            onClick={() => { this.setState({ hasError: false, error: null }); window.location.reload(); }}
            style={{
              padding: "9px 20px", background: "#6366F1", color: "#fff",
              border: "none", borderRadius: 8, cursor: "pointer", fontSize: "0.875rem", fontWeight: 600,
            }}
          >
            Reload App
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

const statusMatch = window.location.pathname.match(/^\/status\/?(.*)$/);

export default function App() {
  if (statusMatch) {
    const tenant = statusMatch[1] || "default";
    return (
      <ErrorBoundary>
        <PublicStatusPage tenant={tenant} />
      </ErrorBoundary>
    );
  }
  return (
    <ErrorBoundary>
      <AppShell />
    </ErrorBoundary>
  );
}
