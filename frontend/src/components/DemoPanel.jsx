import { useState } from "react";
import { fetchJson } from "../api/http";

function DemoPanel({ onScenarioRun }) {
  const [loadingScenario, setLoadingScenario] = useState("");
  const [scenarioError, setScenarioError] = useState("");

  const scenarios = [
    {
      id: "checkout_timeout",
      label: "Checkout Timeout Cascade",
      description: "Latency → timeout → timeout spike on checkout-api",
    },
    {
      id: "payments_database",
      label: "Payments Database Failure",
      description: "Database outage impacting payments-api",
    },
    {
      id: "inventory_degradation",
      label: "Inventory Service Degradation",
      description: "Latency → timeout → failure spike on inventory-api",
    },
  ];

  async function runScenario(scenarioId) {
    try {
      setLoadingScenario(scenarioId);

      await fetchJson("http://localhost:8080/api/v1/demo/scenario", {
        method: "POST",
        body: JSON.stringify({
          scenario: scenarioId,
        }),
      });

      if (onScenarioRun) {
        await onScenarioRun();
      }
    } catch (error) {
      console.error("Failed to run scenario:", error);
      setScenarioError(error.message || "Failed to run demo scenario");
    } finally {
      setLoadingScenario("");
    }
  }

  async function resetSystem() {
  try {
    await fetchJson("http://localhost:8080/api/v1/dev/reset", {
      method: "POST",
    });

    if (onScenarioRun) {
      await onScenarioRun();
    }
  } catch (error) {
    console.error("Reset failed:", error);
  }
}

  return (
    <section className="panel">
      <p className="panel-eyebrow">Demo Mode</p>
      <h2 className="panel-title">Scenario Launcher</h2>
      {scenarioError && (
        <div style={{ marginBottom: "0.75rem", padding: "0.5rem 0.75rem", borderRadius: 8, fontSize: "0.85rem", background: "rgba(127,29,29,0.25)", border: "1px solid rgba(248,113,113,0.3)", color: "#fca5a5" }}>
          {scenarioError}
        </div>
      )}
      <div className="demo-scenario-list">
        {scenarios.map((scenario) => (
          <div key={scenario.id} className="demo-scenario-card">
            <strong>{scenario.label}</strong>
            <p>{scenario.description}</p>
            <button
              type="button"
              className="primary-button"
              onClick={() => runScenario(scenario.id)}
              disabled={loadingScenario === scenario.id}
            >
              {loadingScenario === scenario.id ? "Running..." : "Run Scenario"}
            </button>

            <button className="secondary-button" onClick={resetSystem}>
  Reset System
</button>
          </div>
        ))}
      </div>
    </section>
  );
}

export default DemoPanel;
