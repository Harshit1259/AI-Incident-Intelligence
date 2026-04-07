import { useState } from "react";
import { fetchJson } from "../api/http";

function DemoPanel({ onScenarioRun }) {
  const [loadingScenario, setLoadingScenario] = useState("");

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
      window.alert(error.message || "Failed to run demo scenario");
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
