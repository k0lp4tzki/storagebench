import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useLive } from "../state/LiveProvider";
import { api } from "../api/client";
import type { Batch, RunSummary } from "../api/types";
import { Sparkline } from "../components/Sparkline";
import { StatusBadge, RunStatusBadge } from "../components/StatusBadge";
import { fmtIOPS, fmtLatUs, fmtTime } from "../format";

export function Dashboard() {
  const { agents, liveMetrics, testStates } = useLive();
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      api.runs().then((r) => active && setRuns((r ?? []).slice(0, 6))).catch((e) => active && setErr(String(e)));
      api.batches().then((b) => active && setBatches((b ?? []).slice(0, 5))).catch(() => {});
    };
    load();
    const t = setInterval(load, 8000);
    return () => {
      active = false;
      clearInterval(t);
    };
  }, []);

  const online = agents.filter((a) => a.status === "online").length;
  const activeBatches = batches.filter((b) => b.status === "running");

  return (
    <div>
      {/* Top stats */}
      <section className="grid cols-3 section">
        <div className="card stat">
          <div className="label">Hosts online</div>
          <div className="value">
            {online}
            <span className="unit">/ {agents.length}</span>
          </div>
          <div className="sub">verbundene Agenten</div>
        </div>
        <div className="card stat">
          <div className="label">Aktive Profilläufe</div>
          <div className="value">{activeBatches.length}</div>
          <div className="sub">laufende Batches</div>
        </div>
        <div className="card stat">
          <div className="label">Testläufe gesamt</div>
          <div className="value">{runs.length > 0 ? "—" : 0}</div>
          <div className="sub">letzte in der Liste unten</div>
        </div>
      </section>

      {/* Hosts */}
      <section className="section">
        <div className="section-head">
          <h2>Hosts</h2>
          <Link className="btn sm" to="/profiles">
            Profil ausführen
          </Link>
        </div>
        <div className="grid cols-3">
          {agents.length === 0 && (
            <div className="card empty">Noch keine Agenten verbunden.</div>
          )}
          {agents.map((a) => {
            const samples = liveMetrics[a.agentId] ?? [];
            const last = samples[samples.length - 1];
            const iopsSeries = samples.map((s) => (s.ior ?? 0) + (s.iow ?? 0));
            const phase = testStates[a.agentId]?.phase;
            return (
              <div className="card" key={a.agentId}>
                <div className="toolbar" style={{ alignItems: "start" }}>
                  <div>
                    <div style={{ fontWeight: 650, fontSize: 15 }}>
                      {a.system.hostname}
                    </div>
                    <div className="faint" style={{ fontSize: 12, marginTop: 2 }}>
                      {a.system.distro ?? a.system.os} · {a.system.arch}
                      {a.capabilities.fio
                        ? ` · fio ${a.capabilities.fioVersion ?? ""}`
                        : " · kein fio"}
                    </div>
                  </div>
                  <div className="spacer" />
                  <StatusBadge status={a.status} />
                </div>

                <div style={{ margin: "14px 0 6px" }}>
                  <Sparkline values={iopsSeries} color="var(--accent)" height={40} />
                </div>

                <div className="row" style={{ marginTop: 8 }}>
                  <div>
                    <div className="label faint" style={{ fontSize: 11 }}>
                      IOPS
                    </div>
                    <div style={{ fontWeight: 650, fontVariantNumeric: "tabular-nums" }}>
                      {fmtIOPS((last?.ior ?? 0) + (last?.iow ?? 0))}
                    </div>
                  </div>
                  <div>
                    <div className="label faint" style={{ fontSize: 11 }}>
                      Latenz p99
                    </div>
                    <div style={{ fontWeight: 650, fontVariantNumeric: "tabular-nums" }}>
                      {fmtLatUs(last?.l99)}
                    </div>
                  </div>
                  <div>
                    <div className="label faint" style={{ fontSize: 11 }}>
                      Phase
                    </div>
                    <div style={{ fontWeight: 600 }}>{phase ?? "idle"}</div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {/* Active batches */}
      {activeBatches.length > 0 && (
        <section className="section">
          <h2>Laufende Profilläufe</h2>
          <div className="grid cols-2">
            {activeBatches.map((b) => (
              <Link
                to={`/batches/${b.id}`}
                key={b.id}
                className="card"
                style={{ textDecoration: "none", color: "inherit" }}
              >
                <div className="toolbar">
                  <div style={{ fontWeight: 600 }}>{b.name}</div>
                  <div className="spacer" />
                  <RunStatusBadge status={b.status} />
                </div>
                <div className="faint" style={{ fontSize: 12, margin: "8px 0" }}>
                  Schritt {Math.min(b.stepIndex + 1, b.totalSteps)} von {b.totalSteps} ·{" "}
                  {b.agentIds.length} Host(s)
                </div>
                <div className="progress">
                  <span
                    style={{
                      width: `${((b.stepIndex + 1) / Math.max(1, b.totalSteps)) * 100}%`,
                    }}
                  />
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}

      {/* Recent runs */}
      <section className="section">
        <div className="section-head">
          <h2>Letzte Testläufe</h2>
          <Link className="link" to="/results">
            alle ansehen →
          </Link>
        </div>
        {err && <div className="error">{err}</div>}
        <div className="card pad-0">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Status</th>
                <th>Gestartet</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {runs.length === 0 && (
                <tr>
                  <td colSpan={4} className="empty">
                    Noch keine Läufe.
                  </td>
                </tr>
              )}
              {runs.map((r) => (
                <tr key={r.id}>
                  <td>{r.name || r.id}</td>
                  <td>
                    <RunStatusBadge status={r.status} />
                  </td>
                  <td className="muted">{fmtTime(r.startedAt)}</td>
                  <td>
                    <Link className="link" to={`/runs/${r.id}`}>
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
