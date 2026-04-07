import React from "react";

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      hasError: false,
      errorMessage: "",
    };
  }

  static getDerivedStateFromError(error) {
    return {
      hasError: true,
      errorMessage: error?.message || "Unknown UI error",
    };
  }

  componentDidCatch(error, errorInfo) {
    console.error("UI crash captured by ErrorBoundary:", error, errorInfo);
  }

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="page-shell">
          <section className="panel detail-panel empty-panel">
            <p className="panel-eyebrow">UI Failure</p>
            <h2 className="panel-title">Something broke in the frontend</h2>
            <p className="hero-copy">
              The UI hit a render failure and stopped safely instead of crashing silently.
            </p>
            <p className="hero-copy">
              Error: {this.state.errorMessage}
            </p>
            <button
              type="button"
              className="primary-button"
              onClick={this.handleReload}
            >
              Reload app
            </button>
          </section>
        </div>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
