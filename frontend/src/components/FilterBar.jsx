function FilterBar({ filters, onChange, onClear, serviceOptions }) {
  return (
    <section className="panel filter-panel">
      <div className="panel-header-row">
        <div>
          <p className="panel-eyebrow">Incident Query Layer</p>
          <h2 className="panel-title">Filter incidents</h2>
        </div>
        <button className="secondary-button" type="button" onClick={onClear}>
          Clear filters
        </button>
      </div>

      <div className="filter-grid">
        <label className="field">
          <span>Search</span>
          <input
            type="text"
            value={filters.search}
            placeholder="Title or service"
            onChange={(event) => onChange("search", event.target.value)}
          />
        </label>

        <label className="field">
          <span>Status</span>
          <select value={filters.status} onChange={(event) => onChange("status", event.target.value)}>
            <option value="">All statuses</option>
            <option value="open">Open</option>
            <option value="acknowledged">Acknowledged</option>
            <option value="resolved">Resolved</option>
          </select>
        </label>

        <label className="field">
          <span>Severity</span>
          <select value={filters.severity} onChange={(event) => onChange("severity", event.target.value)}>
            <option value="">All severities</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </select>
        </label>

        <label className="field">
          <span>Service</span>
          <select value={filters.service} onChange={(event) => onChange("service", event.target.value)}>
            <option value="">All services</option>
            {serviceOptions.map((service) => (
              <option key={service} value={service}>
                {service}
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span>From</span>
          <input type="date" value={filters.from} onChange={(event) => onChange("from", event.target.value)} />
        </label>

        <label className="field">
          <span>To</span>
          <input type="date" value={filters.to} onChange={(event) => onChange("to", event.target.value)} />
        </label>
      </div>
    </section>
  );
}

export default FilterBar;
