export function LoadingState({ label }) {
  return <div className="panel state-panel">{label}</div>;
}

export function ErrorState({ message, onRetry }) {
  return (
    <div className="panel state-panel error-panel">
      <p>{message}</p>
      {onRetry ? (
        <button type="button" className="secondary-button" onClick={onRetry}>
          Retry
        </button>
      ) : null}
    </div>
  );
}

export function EmptyState({ title, subtitle }) {
  return (
    <div className="panel state-panel">
      <h3>{title}</h3>
      <p>{subtitle}</p>
    </div>
  );
}
