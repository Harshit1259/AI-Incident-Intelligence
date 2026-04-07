import React, { useEffect, useState } from "react";

export default function IncidentDetail({ id }) {
  const [data, setData] = useState(null);

  useEffect(() => {
    fetch(`/api/incidents/${id}`)
      .then(res => res.json())
      .then(setData);
  }, [id]);

  if (!data) return <div>Loading...</div>;

  return (
    <div style={{ padding: 20 }}>

      <h2>{data.summary}</h2>
      <p>Confidence: {data.confidence}</p>

      <h3>Root Cause</h3>
      <p>{data.root_cause}</p>

      <h3>Graph</h3>
      <pre>{JSON.stringify(data.graph, null, 2)}</pre>

      <h3>Actions</h3>
      {data.actions.map((a, i) => (
        <div key={i}>
          <b>{a.title}</b>
          <ul>
            {a.steps?.map((s, j) => <li key={j}>{s}</li>)}
          </ul>
        </div>
      ))}

      <h3>Pattern</h3>
      {data.pattern && (
        <div>
          Seen {data.pattern.count} times <br />
          Resolution: {data.pattern.resolution}
        </div>
      )}
    </div>
  );
}