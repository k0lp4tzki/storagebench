import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type {
  EventRecord,
  MetricSample,
  RunDetail,
  StartRunRequest,
} from "../api/types";
import { Chart } from "../components/Chart";
import { RunStatusBadge } from "../components/StatusBadge";
import { fmtBW, fmtIOPS, fmtLatUs, fmtTime, secondsFromStart } from "../format";

const ACTIVE = new Set(["running", "assigned", "starting", "pending"]);

export function RunDetailPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const [run, setRun] = useState<RunDetail | null>(null);
  const [metrics, setMetrics] = useState<Record<string, MetricSample[]>>({});
  const [events, setEvents] = useState<EventRecord[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [d, m, e] = await Promise.all([
        api.run(id),
        api.runMetrics(id),
        api.runEvents(id),
      ]);
      setRun(d);
      setMetrics(m ?? {});
      setEvents(e ?? []);
    } catch (ex) {
      setErr(String(ex));
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!run || !ACTIVE.has(run.status)) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [run, load]);

  async function doStop(abort: boolean) {
    setBusy(true);
    setErr(null);
    try {
      if (abort) await api.abortRun(id);
      else await api.stopRun(id);
      await load();
    } catch (ex) {
      setErr(String(ex));
    } finally {
      setBusy(false);
    }
  }

  async function doRestart() {
    if (!run?.resolvedConfig) return;
    setBusy(true);
    setErr(null);
    try {
      const req: StartRunRequest = {
        name: run.name || "Neustart",
        specs: run.resolvedConfig,
      };
      const { runId } = await api.startRun(req);
      nav(`/runs/${runId}`);
    } catch (ex) {
      setErr(String(ex));
      setBusy(false);
    }
  }

  if (err && !run) return <div className="error">{err}</div>;
  if (!run) return <div className="card empty">Lädt…</div>;

  const active = ACTIVE.has(run.status);
  const nodes = run.nodes ?? [];
  const eventList = events ?? [];

  return (
    <div>
      <div className="section-head">
        <div>
          <h2 style={{ marginBottom: 6 }}>{run.name || run.id}</h2>
          <div className="toolbar" style={{ gap: 8 }}>
            <RunStatusBadge status={run.status} />
            <span className="faint" style={{ fontSize: 12 }}>
              {fmtTime(run.startedAt)}
              {run.endedAt ? ` – ${fmtTime(run.endedAt)}` : ""}
            </span>
          </div>
        </div>
        <div className="toolbar">
          {active ? (
            <>
              <button className="btn secondary" disabled={busy} onClick={() => doStop(false)}>
                Stop
              </button>
              <button className="btn danger" disabled={busy} onClick={() => doStop(true)}>
                Abbrechen
              </button>
            </>
          ) : (
            run.resolvedConfig && (
              <button className="btn" disabled={busy} onClick={doRestart}>
                ▶ Neu starten
              </button>
            )
          )}
        </div>
      </div>

      {err && <div className="error" style={{ marginBottom: 14 }}>{err}</div>}

      {/* Comparison */}
      <section className="section">
        <h2>Vergleich</h2>
        <div className="card pad-0">
          <table>
            <thead>
              <tr>
                <th>Host</th>
                <th>Phase</th>
                <th>IOPS R</th>
                <th>IOPS W</th>
                <th>BW R</th>
                <th>BW W</th>
                <th>Lat Ø</th>
                <th>p95</th>
                <th>p99</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((n) => {
                const f = n.fioSummary;
                return (
                  <tr key={n.agentId}>
                    <td style={{ fontWeight: 550 }}>{n.hostname}</td>
                    <td>
                      <RunStatusBadge status={n.phase} />
                    </td>
                    <td>{fmtIOPS(f?.iopsRead)}</td>
                    <td>{fmtIOPS(f?.iopsWrite)}</td>
                    <td>{fmtBW(f?.bwReadKBs)}</td>
                    <td>{fmtBW(f?.bwWriteKBs)}</td>
                    <td>{fmtLatUs(f?.latMeanUs)}</td>
                    <td>{fmtLatUs(f?.latP95Us)}</td>
                    <td>{fmtLatUs(f?.latP99Us)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Time series */}
      <section className="section">
        <h2>Zeitverlauf</h2>
        <div className="grid cols-2">
          {nodes.map((n) => {
            const s = metrics[n.agentId] ?? [];
            const x = secondsFromStart(s.map((v) => v.t));
            return (
              <div className="card" key={n.agentId}>
                <div style={{ fontWeight: 600, marginBottom: 8 }}>{n.hostname}</div>
                <div className="faint" style={{ fontSize: 11 }}>IOPS</div>
                <Chart
                  x={x}
                  height={160}
                  yFormat={(v) => fmtIOPS(v)}
                  series={[
                    { label: "Read", data: s.map((v) => v.ior ?? 0), color: "var(--read)" },
                    { label: "Write", data: s.map((v) => v.iow ?? 0), color: "var(--write)" },
                  ]}
                />
                <div className="faint" style={{ fontSize: 11, marginTop: 8 }}>
                  Latenz (µs)
                </div>
                <Chart
                  x={x}
                  height={160}
                  yFormat={(v) => fmtLatUs(v)}
                  series={[
                    { label: "Ø", data: s.map((v) => v.lm ?? 0), color: "var(--accent)" },
                    { label: "p99", data: s.map((v) => v.l99 ?? 0), color: "var(--err)" },
                  ]}
                />
              </div>
            );
          })}
        </div>
      </section>

      {/* Events */}
      <section className="section">
        <h2>Ereignisse</h2>
        <div className="card pad-0">
          <table>
            <thead>
              <tr>
                <th>Zeit</th>
                <th>Host</th>
                <th>Schwere</th>
                <th>Quelle</th>
                <th>Nachricht</th>
              </tr>
            </thead>
            <tbody>
              {eventList.length === 0 && (
                <tr>
                  <td colSpan={5} className="empty">
                    Keine Ereignisse.
                  </td>
                </tr>
              )}
              {eventList.map((e, i) => (
                <tr key={i}>
                  <td className="muted">
                    {new Date(e.tsWallMilli).toLocaleTimeString("de-DE")}
                  </td>
                  <td>{e.agentId}</td>
                  <td>
                    <span
                      className={`badge ${
                        e.severity === "error" ? "err" : e.severity === "warn" ? "warn" : ""
                      }`}
                    >
                      {e.severity}
                    </span>
                  </td>
                  <td className="muted">{e.source}</td>
                  <td>{e.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
