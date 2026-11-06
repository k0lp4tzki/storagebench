import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { RunSummary } from "../api/types";
import { fmtTime } from "../format";

export function Results() {
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .runs()
      .then(setRuns)
      .catch((e) => setErr(String(e)))
      .finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <h1>Ergebnisse</h1>
      {err && <div className="error">{err}</div>}
      <div className="card">
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
            {loading && (
              <tr>
                <td colSpan={5} className="muted">
                  Lädt…
                </td>
              </tr>
            )}
            {!loading && runs.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  Noch keine Läufe.
                </td>
              </tr>
            )}
            {runs.map((r) => (
              <tr key={r.id}>
                <td>{r.name || r.id}</td>
                <td>{r.status}</td>
                <td>{fmtTime(r.startedAt)}</td>
                <td>{fmtTime(r.endedAt)}</td>
                <td>
                  <Link className="rowlink" to={`/runs/${r.id}`}>
                    Details
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
