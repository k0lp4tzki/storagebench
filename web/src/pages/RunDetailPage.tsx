import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type {
  EventRecord,
  MetricSample,
  RunDetail,
  StartRunRequest,
} from "../api/types";
import { TimeChart } from "../components/TimeChart";
import { fmtBW, fmtIOPS, fmtLatUs, fmtTime } from "../format";

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

  // Poll while the run is active so live persisted data refreshes.
  useEffect(() => {
    if (!run || !ACTIVE.has(run.status)) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [run, load]);

  async function doStop(abort: boolean) {
    setBusy(true);
    setErr(null);
    try {
      abort ? await api.abortRun(id) : await api.stopRun(id);
      await load();
    } catch (ex) {
      setErr(String(ex));
    } finally {
      setBusy(false);
    }
  }

  // Restart re-runs the same frozen config as a new run and navigates to it.
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
  if (!run) return <div className="muted">Lädt…</div>;

  const active = ACTIVE.has(run.status);
  // Go serializes empty slices as null; guard every list access.
  const nodes = run.nodes ?? [];
  const eventList = events ?? [];

  return (
    <div>
      <h1>{run.name || run.id}</h1>
      <div className="inline" style={{ marginBottom: 16 }}>
        <span className="badge">Status: {run.status}</span>
        <span className="badge">Start: {fmtTime(run.startedAt)}</span>
        <span className="badge">Ende: {fmtTime(run.endedAt)}</span>
        {active && (
          <>
            <button
              className="btn secondary"
              disabled={busy}
              onClick={() => doStop(false)}
            >
              Stop
            </button>
            <button
              className="btn danger"
              disabled={busy}
              onClick={() => doStop(true)}
            >
              Abbrechen
            </button>
          </>
        )}
        {!active && run.resolvedConfig && (
          <button className="btn" disabled={busy} onClick={doRestart}>
            Neu starten
          </button>
        )}
      </div>
      {err && <div className="error">{err}</div>}

      <section style={{ marginBottom: 24 }}>
        <h2>Vergleich</h2>
        <div className="card">
          <table>
            <thead>
              <tr>
                <th>Server</th>
                <th>Phase</th>
                <th>IOPS R</th>
                <th>IOPS W</th>
                <th>BW R</th>
                <th>BW W</th>
                <th>Latenz Ø</th>
                <th>p95</th>
                <th>p99</th>
              </tr>
            </thead>
            <tbody>
              {nodes.map((n) => {
                const f = n.fioSummary;
                return (
                  <tr key={n.agentId}>
                    <td>{n.hostname}</td>
                    <td>{n.phase}</td>
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

      <section style={{ marginBottom: 24 }}>
        <h2>Zeitverlauf</h2>
        <div className="grid3">
          {nodes.map((n) => {
            const s = metrics[n.agentId] ?? [];
            return (
              <div className="card" key={n.agentId}>
                <div className="host">{n.hostname}</div>
                <div style={{ marginTop: 12 }}>
                  <TimeChart
                    series={[
                      {
                        values: s.map((x) => x.ior ?? 0),
                        color: "var(--read)",
                        label: "IOPS Read",
                      },
                      {
                        values: s.map((x) => x.iow ?? 0),
                        color: "var(--write)",
                        label: "IOPS Write",
                      },
                    ]}
                  />
                </div>
                <div style={{ marginTop: 12 }}>
                  <TimeChart
                    unit="µs"
                    series={[
                      {
                        values: s.map((x) => x.lm ?? 0),
                        color: "var(--accent)",
                        label: "Latenz Ø",
                      },
                      {
                        values: s.map((x) => x.l99 ?? 0),
                        color: "var(--err)",
                        label: "Latenz p99",
                      },
                    ]}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </section>

      <section>
        <h2>Ereignisse</h2>
        <div className="card">
          <table>
            <thead>
              <tr>
                <th>Zeit</th>
                <th>Server</th>
                <th>Schwere</th>
                <th>Quelle</th>
                <th>Nachricht</th>
              </tr>
            </thead>
            <tbody>
              {eventList.length === 0 && (
                <tr>
                  <td colSpan={5} className="muted">
                    Keine Ereignisse.
                  </td>
                </tr>
              )}
              {eventList.map((e, i) => (
                <tr key={i}>
                  <td>{new Date(e.tsWallMilli).toLocaleTimeString("de-DE")}</td>
                  <td>{e.agentId}</td>
                  <td>{e.severity}</td>
                  <td>{e.source}</td>
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
