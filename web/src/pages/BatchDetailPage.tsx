import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../api/client";
import { useLive } from "../state/LiveProvider";
import type { BatchDetail } from "../api/types";
import { RunStatusBadge } from "../components/StatusBadge";
import { fmtTime } from "../format";

const ACTIVE = new Set(["running", "created"]);

export function BatchDetailPage() {
  const { id = "" } = useParams();
  const { batchProgress } = useLive();
  const [batch, setBatch] = useState<BatchDetail | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setBatch(await api.batch(id));
    } catch (ex) {
      setErr(String(ex));
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  // Refresh on live batch progress for this batch, and poll while active.
  const live = batchProgress[id];
  useEffect(() => {
    if (live) load();
  }, [live, load]);

  useEffect(() => {
    if (!batch || !ACTIVE.has(batch.status)) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [batch, load]);

  if (err && !batch) return <div className="error">{err}</div>;
  if (!batch) return <div className="card empty">Lädt…</div>;

  const runs = batch.runs ?? [];
  const pct = (Math.min(batch.stepIndex + 1, batch.totalSteps) / Math.max(1, batch.totalSteps)) * 100;

  return (
    <div>
      <div className="section-head">
        <div>
          <h2 style={{ marginBottom: 6 }}>{batch.name || batch.id}</h2>
          <div className="toolbar" style={{ gap: 8 }}>
            <RunStatusBadge status={batch.status} />
            <span className="faint" style={{ fontSize: 12 }}>
              {batch.profileName ? `Profil: ${batch.profileName} · ` : ""}
              {batch.agentIds.length} Host(s) · {fmtTime(batch.startedAt)}
            </span>
          </div>
        </div>
      </div>

      {/* Progress */}
      <div className="card section">
        <div className="toolbar" style={{ marginBottom: 10 }}>
          <div style={{ fontWeight: 600 }}>Fortschritt</div>
          <div className="spacer" />
          <span className="muted">
            Schritt {Math.min(batch.stepIndex + 1, batch.totalSteps)} von {batch.totalSteps}
          </span>
        </div>
        <div className="progress">
          <span style={{ width: `${pct}%` }} />
        </div>
      </div>

      {/* Steps / runs */}
      <section className="section">
        <h2>Schritte</h2>
        <div className="card pad-0">
          <table>
            <thead>
              <tr>
                <th>#</th>
                <th>Schritt</th>
                <th>Status</th>
                <th>Gestartet</th>
                <th>Beendet</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {runs.length === 0 && (
                <tr>
                  <td colSpan={6} className="empty">
                    Noch keine Schritte gestartet.
                  </td>
                </tr>
              )}
              {runs.map((r) => (
                <tr key={r.runId}>
                  <td className="muted">{r.step + 1}</td>
                  <td style={{ fontWeight: 550 }}>{r.name}</td>
                  <td>
                    <RunStatusBadge status={r.status} />
                  </td>
                  <td className="muted">{fmtTime(r.startedAt)}</td>
                  <td className="muted">{fmtTime(r.endedAt)}</td>
                  <td>
                    <Link className="link" to={`/runs/${r.runId}`}>
                      Details
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
