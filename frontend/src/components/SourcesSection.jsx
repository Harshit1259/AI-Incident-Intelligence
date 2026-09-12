/**
 * SourcesSection — connect a tool and manage its sources, inside the
 * Integrations page.
 *
 * A source is the token a tool sends so NeuroOps knows which tenant the data
 * belongs to. The token is shown once, when the source is created or its
 * token is rotated; the parent fills it into the setup snippet via
 * onTokenRevealed. Operators and admins can connect, test, rotate and delete;
 * viewers see the list only.
 */
import { useCallback, useEffect, useState } from "react";
import { Check, Copy, KeyRound, Plus, RefreshCw, Send, Trash2 } from "lucide-react";

import {
  createSource, deleteSource, listSources, rotateSourceToken, sendSourceTest,
} from "../api/sources.js";

function ago(iso) {
  if (!iso) return "never";
  const s = Math.max(0, Math.round((Date.now() - Date.parse(iso)) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  if (s < 172800) return `${Math.round(s / 3600)}h ago`;
  return `${Math.round(s / 86400)}d ago`;
}

const HEALTH_TONE = { healthy: "tone-success", degraded: "tone-warning", error: "tone-danger", waiting: "tone-neutral" };
const HEALTH_LABEL = { waiting: "waiting for data" };

function TokenReveal({ source, onDismiss }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    navigator.clipboard.writeText(source.token).catch(() => {});
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div className="source-token" role="status">
      <p className="source-token-title">
        <KeyRound size={13} /> Token for “{source.name}” — copy it now, it won’t be shown again
      </p>
      <div className="source-token-row">
        <code className="source-token-value">{source.token}</code>
        <button type="button" className="btn btn-outline btn-xs" onClick={copy}>
          {copied ? <Check size={11} /> : <Copy size={11} />} {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <p className="form-hint">The setup below now has this token filled in.</p>
      <button type="button" className="btn btn-ghost btn-xs" onClick={onDismiss}>I’ve saved it</button>
    </div>
  );
}

export default function SourcesSection({ token, type, integrationName, canManage, onTokenRevealed }) {
  const [sources, setSources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState("");
  const [revealed, setRevealed] = useState(null); // source incl. token, shown once
  const [testResult, setTestResult] = useState({}); // id → message
  const [confirmDelete, setConfirmDelete] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      const all = await listSources(token);
      setSources(all.filter((s) => s.type === type));
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [token, type]);

  useEffect(() => {
    load();
  }, [load]);

  function reveal(src) {
    setRevealed(src);
    onTokenRevealed?.(src.token);
  }

  function hideToken() {
    setRevealed(null);
    onTokenRevealed?.(null);
  }

  async function handleCreate(e) {
    e.preventDefault();
    setBusy("create");
    setError("");
    try {
      const src = await createSource(token, name.trim() || `${integrationName}`, type);
      setName("");
      reveal(src);
      await load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy("");
    }
  }

  async function handleRotate(src) {
    setBusy(`rotate-${src.id}`);
    setError("");
    try {
      reveal(await rotateSourceToken(token, src.id));
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy("");
    }
  }

  async function handleDelete(src) {
    setBusy(`delete-${src.id}`);
    setError("");
    try {
      await deleteSource(token, src.id);
      if (revealed?.id === src.id) hideToken();
      setConfirmDelete("");
      await load();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy("");
    }
  }

  async function handleTest(src) {
    setBusy(`test-${src.id}`);
    setTestResult((r) => ({ ...r, [src.id]: "" }));
    try {
      const res = await sendSourceTest(token, src.id);
      const message = type === "grafana"
        ? "✔ “[Test] Grafana contact point test” incident created — check Incidents; it closes itself in a few minutes"
        : `✔ Accepted (${res.processed} event${res.processed === 1 ? "" : "s"}) — check Incidents`;
      setTestResult((r) => ({ ...r, [src.id]: message }));
      await load();
    } catch (err) {
      setTestResult((r) => ({ ...r, [src.id]: `✖ ${err.message}` }));
    } finally {
      setBusy("");
    }
  }

  return (
    <div className="detail-section is-divided sources-section">
      <h4 className="detail-section-label">Connection</h4>

      {canManage ? (
        <form className="source-create" onSubmit={handleCreate}>
          <label className="sr-only" htmlFor={`source-name-${type}`}>Source name</label>
          <input
            id={`source-name-${type}`}
            className="form-input"
            value={name}
            maxLength={100}
            placeholder={`Name, e.g. prod-${type}`}
            onChange={(e) => setName(e.target.value)}
          />
          <button type="submit" className="btn btn-primary btn-sm" disabled={busy === "create"}>
            <Plus size={12} /> Connect {integrationName}
          </button>
        </form>
      ) : (
        <p className="form-hint">Ask an operator or admin to connect {integrationName}.</p>
      )}

      {error && <p className="form-error">{error}</p>}
      {revealed && <TokenReveal source={revealed} onDismiss={hideToken} />}

      {loading ? (
        <p className="form-hint">Loading sources…</p>
      ) : sources.length === 0 ? (
        <p className="form-hint">No {integrationName} source yet.</p>
      ) : (
        <ul className="source-list">
          {sources.map((src) => (
            <li key={src.id} className="source-item">
              <div className="source-item-main">
                <span className="source-item-name">{src.name}</span>
                <span className="source-item-meta">
                  last data {ago(src.last_event_at)} · {src.total_events || 0} events
                  {src.last_error ? ` · last error: ${src.last_error}` : ""}
                </span>
                {testResult[src.id] && <span className="source-item-test">{testResult[src.id]}</span>}
              </div>
              <span className={`pill is-plain ${HEALTH_TONE[src.health_score] || "tone-neutral"}`}>
                {HEALTH_LABEL[src.health_score] || src.health_score || src.status || "new"}
              </span>
              {canManage && (
                <div className="source-item-actions">
                  <button type="button" className="btn btn-ghost btn-xs" onClick={() => handleTest(src)} disabled={busy !== ""}>
                    <Send size={11} /> Test
                  </button>
                  <button type="button" className="btn btn-ghost btn-xs" onClick={() => handleRotate(src)} disabled={busy !== ""}>
                    <RefreshCw size={11} /> Rotate token
                  </button>
                  {confirmDelete === src.id ? (
                    <>
                      <button type="button" className="btn btn-danger btn-xs" onClick={() => handleDelete(src)} disabled={busy !== ""}>
                        Delete — its token stops working
                      </button>
                      <button type="button" className="btn btn-ghost btn-xs" onClick={() => setConfirmDelete("")}>Cancel</button>
                    </>
                  ) : (
                    <button type="button" className="btn btn-ghost btn-xs" onClick={() => setConfirmDelete(src.id)} disabled={busy !== ""}>
                      <Trash2 size={11} /> Delete
                    </button>
                  )}
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
