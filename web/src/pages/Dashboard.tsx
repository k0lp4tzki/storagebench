import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useLive } from "../state/LiveProvider";
import { api } from "../api/client";
import type { RunSummary } from "../api/types";
import { Sparkline } from "../components/Sparkline";
import { StatusBadge } from "../components/StatusBadge";
import { fmtIOPS, fmtLatUs, fmtTime } from "../format";

export function Dashboard() {
  const { agents, liveMetrics, testStates } = useLive();
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const load = () =>
      api
        .runs()
        .then((r) => active && setRuns(r.slice(0, 8)))
        .catch((e) => active && setErr(String(e)));
    load();
    const t = setInterval(load, 10000);
    return () => {
      active = false;
      clearInterval(t);
    };
  }, []);

  return (
    <div>
      <h1>Dashboard</h1>

      <section className="grid3">
        {agents.length === 0 && (
          <div className="card muted">Noch keine Agenten verbunden.</div>
        )}
        {agents.map((a) => {
          const samples = liveMetrics[a.agentId] ?? [];
          const last = samples[samples.length - 1];
          const iopsSeries = samples.map((s) => (s.ior ?? 0) + (s.iow ?? 0));
          const phase = testStates[a.agentId]?.phase;
          return (
            <div className="card" key={a.agentId}>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "start",
                }}
              >
                <div>
                  <div className="host">{a.system.hostname}</div>
                  <div className="sub">
                    {a.system.distro ?? a.system.os} · {a.system.arch}
                    {a.capabilities.fio
                      ? ` · fio ${a.capabilities.fioVersion ?? ""}`
                      : " · kein fio"}
                  </div>
                </div>
                <StatusBadge status={a.status} />
              </div>

              <div style={{ marginTop: 10 }}>
                <Sparkline values={iopsSeries} />
              </div>

              <div className="metrics-row">
                <div className="metric">
                  <div className="label">IOPS</div>
                  <div className="value">
                    {fmtIOPS((last?.ior ?? 0) + (last?.iow ?? 0))}
                  </div>
                </div>
                <div className="metric">
                  <div className="label">Latenz p99</div>
                  <div className="value">{fmtLatUs(last?.l99)}</div>
                </div>
                <div className="metric">
                  <div className="label">Phase</div>
                  <div className="value" style={{ fontSize: 14 }}>
                    {phase ?? "idle"}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </section>

      <section style={{ marginTop: 24 }}>
        <h2>Letzte Testläufe</h2>
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
              {runs.length === 0 && (
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
      </section>
    </div>
  );
}
