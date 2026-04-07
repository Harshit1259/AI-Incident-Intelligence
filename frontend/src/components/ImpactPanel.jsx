function ImpactPanel({ impact }) {
  if (!impact) {
    return null;
  }

  return (
    <div className="detail-section">
      <h3>Impact Analysis</h3>

      <p>
        <strong>Primary Service:</strong> {impact.primary_service}
      </p>

      <p>
        <strong>Impact Level:</strong>{" "}
        <span className={`impact-${impact.impact_level}`}>
          {impact.impact_level.toUpperCase()}
        </span>
      </p>

      <p>
        <strong>Downstream Services:</strong>
      </p>
      <ul>
        {(impact.downstream || []).map((svc) => (
          <li key={svc}>{svc}</li>
        ))}
      </ul>

      <p>
        <strong>Affected Services:</strong>
      </p>
      <ul>
        {(impact.affected_services || []).map((svc) => (
          <li key={svc}>{svc}</li>
        ))}
      </ul>
    </div>
  );
}

export default ImpactPanel;
