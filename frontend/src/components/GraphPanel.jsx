import React from "react";

export default function GraphPanel({ graph }) {
  const nodes = graph?.nodes || [];
  const edges = graph?.edges || [];

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Service Graph</h3>
      </div>

      {nodes.length === 0 ? (
        <div className="empty-state">No graph data available.</div>
      ) : (
        <div className="graph-panel">
          <div className="graph-section">
            <div className="graph-section-title">Nodes</div>
            <div className="graph-list">
              {nodes.map((node) => (
                <div key={node.id} className="graph-node-card">
                  <div className="graph-node-title">{node.label || node.id}</div>
                  <div className="graph-node-meta">
                    <span>{node.node_type || "-"}</span>
                    {node.severity ? <span>{node.severity}</span> : null}
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="graph-section">
            <div className="graph-section-title">Relationships</div>
            <div className="graph-list">
              {edges.map((edge, index) => (
                <div key={`${edge.from}-${edge.to}-${index}`} className="graph-edge-card">
                  <strong>{edge.from}</strong>
                  <span className="graph-arrow">→</span>
                  <strong>{edge.to}</strong>
                  <span className="graph-relation">{edge.relation}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
