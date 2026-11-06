import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { Batch, RunSummary } from "../api/types";
import { RunStatusBadge } from "../components/StatusBadge";
import { fmtTime } from "../format";

type Tab = "batches" | "runs";

export function Results() {
  const [tab, setTab] = useState<Tab>("batches");
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [q, setQ] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([api.runs(), api.batches()])
      .then(([r, b]) => {
        setRuns(r ?? []);
        setBatches(b ?? []);
      })
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false));
  }, []);

  const filteredRuns = useMemo(
    () =>
      runs.filter((r) =>
        (r.name + r.id + r.status).toLowerCase().includes(q.toLowerCase()),
      ),
    [runs, q],
  );
  const filteredBatches = useMemo(
    () =>
      batches.filter((b) =>
        (b.name + b.id + b.status).toLowerCase().includes(q.toLowerCase()),
      ),
    [batches, q],
  );

  return (
    <div>
      <div className="toolbar section" style={{ marginBottom: 18 }}>
        <div className="toolbar" style={{ gap: 4 }}>
          <button
            className={`btn sm ${tab === "batches" ? "" : "ghost"}`}
            onClick={() => setTab("batches")}
          >
            Profilläufe
          </button>
          <button
            className={`btn sm ${tab === "runs" ? "" : "ghost"}`}
            onClick={() => setTab("runs")}
          >
            Einzelne Läufe
          </button>
        </div>
        <div className="spacer" />
        <input
          placeholder="Suchen…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          style={{ maxWidth: 240 }}
        />
      </div>

      {err && <div className="error">{err}</div>}

      <div className="card pad-0">
        {loading ? (
          <div className="empty">Lädt…</div>
        ) : tab === "batches" ? (
          <table>
            <thead>
              <tr>
                <th>Profillauf</th>
                <th>Status</th>
                <th>Fortschritt</th>
                <th>Hosts</th>
                <th>Gestartet</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filteredBatches.length === 0 && (
                <tr>
                  <td colSpan={6} className="empty">
                    Keine Profilläufe.
                  </td>
                </tr>
              )}
              {filteredBatches.map((b) => (
                <tr key={b.id}>
                  <td style={{ fontWeight: 550 }}>{b.name || b.id}</td>
                  <td>
                    <RunStatusBadge status={b.status} />
                  </td>
                  <td className="muted">
                    {Math.min(b.stepIndex + 1, b.totalSteps)}/{b.totalSteps}
                  </td>
                  <td className="muted">{b.agentIds.length}</td>
                  <td className="muted">{fmtTime(b.startedAt)}</td>
                  <td>
                    <Link className="link" to={`/batches/${b.id}`}>
                      Ansehen
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Status</th>
                <th>Gestartet</th>
                <th>Beendet</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {filteredRuns.length === 0 && (
                <tr>
                  <td colSpan={5} className="empty">
                    Keine Läufe.
                  </td>
                </tr>
              )}
              {filteredRuns.map((r) => (
                <tr key={r.id}>
                  <td style={{ fontWeight: 550 }}>{r.name || r.id}</td>
                  <td>
                    <RunStatusBadge status={r.status} />
                  </td>
                  <td className="muted">{fmtTime(r.startedAt)}</td>
                  <td className="muted">{fmtTime(r.endedAt)}</td>
                  <td>
                    <Link className="link" to={`/runs/${r.id}`}>
                      Details
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
