import { useEffect, useState } from "react";

function App() {
  const [backendStatus, setBackendStatus] = useState("Loading...");

  useEffect(() => {
    fetch("http://localhost:8080/health")
      .then((response) => response.json())
      .then((data) => {
        setBackendStatus(data.status);
      })
      .catch((error) => {
        console.error("Error connecting to backend:", error);
        setBackendStatus("Backend not reachable");
      });
  }, []);

  return (
    <div style={{ padding: "40px", fontFamily: "Arial, sans-serif" }}>
      <h1>AI Incident Platform</h1>
      <p>Backend Status: {backendStatus}</p>
    </div>
  );
}

export default App;
