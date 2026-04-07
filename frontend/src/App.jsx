import React from "react";
import IncidentCommandCenter from "./components/IncidentCommandCenter";
import "./App.css";

// Global error boundary — prevents blank screen on any crash
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
          minHeight: "100vh", padding: "2rem", fontFamily: "system-ui, sans-serif",
          background: "#0f1117", color: "#e5e7eb", textAlign: "center"
        }}>
          <div style={{ fontSize: "3rem", marginBottom: "1rem" }}>⚠️</div>
          <h2 style={{ margin: "0 0 0.5rem" }}>Something went wrong</h2>
          <p style={{ color: "#9ca3af", maxWidth: 500, lineHeight: 1.6, fontSize: "0.9rem" }}>
            {this.state.error?.message || "An unexpected error occurred."}
          </p>
          <button
            onClick={() => { this.setState({ hasError: false, error: null }); window.location.reload(); }}
            style={{
              marginTop: "1.5rem", padding: "0.6rem 1.5rem", background: "#818cf8",
              color: "#fff", border: "none", borderRadius: "8px", cursor: "pointer", fontSize: "0.9rem"
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

export default function App() {
  return (
    <ErrorBoundary>
      <IncidentCommandCenter />
    </ErrorBoundary>
  );
}
