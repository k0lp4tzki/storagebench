import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import type { EventRecord, FioSummary, MetricSample, RunDetail, Spec, StartRunRequest } from "../api/types";
import { Chart, type ChartMarker } from "../components/Chart";
import { RunStatusBadge } from "../components/StatusBadge";
import { Empty, PageHeader } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { ConfirmDialog } from "../components/Modal";
import { FaultPanel } from "../components/FaultPanel";
import { useLive } from "../state/LiveProvider";
import { ACTIVE_RUN, workloadLabel } from "../domain";
import {
  coefficientOfVariation,
  elapsedSec,
  fmtBW,
  fmtDateTime,
  fmtDuration,
  fmtIOPS,
  fmtLatUs,
  fmtPct,
  secondsFromStart,
  shortId,
} from "../format";

// Per-host series colours for overlaid charts. Distinct in hue and lightness.
const HOST_COLORS = ["var(--read)", "var(--write)", "#a78bfa", "#4cc38a", "#f2707a", "#e8b339"];

type Col = { key: keyof FioSummary; label: string; fmt: (v?: number) => string; better: "max" | "min"; sep?: boolean };
const COLS: Col[] = [
  { key: "iopsRead", label: "Read", fmt: fmtIOPS, better: "max", sep: true },
  { key: "iopsWrite", label: "Write", fmt: fmtIOPS, better: "max" },
  { key: "bwReadKBs", label: "Read", fmt: fmtBW, better: "max", sep: true },
  { key: "bwWriteKBs", label: "Write", fmt: fmtBW, better: "max" },
  { key: "latMeanUs", label: "Ø", fmt: fmtLatUs, better: "min", sep: true },
  { key: "latP95Us", label: "p95", fmt: fmtLatUs, better: "min" },
  { key: "latP99Us", label: "p99", fmt: fmtLatUs, better: "min" },
  { key: "latP999Us", label: "p99.9", fmt: fmtLatUs, better: "min" },
];

export function RunDetailPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const { agents } = useLive();
  const [run, setRun] = useState<RunDetail | null>(null);
  const [metrics, setMetrics] = useState<Record<string, MetricSample[]>>({});
  const [events, setEvents] = useState<EventRecord[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmAbort, setConfirmAbort] = useState(false);

  const load = useCallback(async () => {
    try {
      const [d, m, e] = await Promise.all([api.run(id), api.runMetrics(id), api.runEvents(id)]);
      setRun(d);
      setMetrics(m ?? {});
      setEvents(e ?? []);
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    if (!run || !ACTIVE_RUN.has(run.status)) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [run, load]);

  async function doStop(abort: boolean) {
    setBusy(true);
    setErr(null);
    try {
      if (abort) await api.abortRun(id);
      else await api.stopRun(id);
      setConfirmAbort(false);
      await load();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    } finally {
      setBusy(false);
    }
  }

  async function doRestart() {
    if (!run?.resolvedConfig) return;
    setBusy(true);
    setErr(null);
    try {
      const req: StartRunRequest = { name: run.name || "Neustart", specs: run.resolvedConfig };
      const { runId } = await api.startRun(req);
      nav(`/runs/${runId}`);
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
      setBusy(false);
    }
  }

  const nodes = run?.nodes ?? [];
  const hostName = useMemo(() => {
    const m: Record<string, string> = {};
    for (const n of nodes) m[n.agentId] = n.hostname;
    return m;
  }, [nodes]);

  const best = useMemo(() => {
    const b: Partial<Record<keyof FioSummary, number>> = {};
    if (nodes.length < 2) return b;
    for (const c of COLS) {
      const vals = nodes.map((n) => n.fioSummary?.[c.key]).filter((v): v is number => typeof v === "number" && v > 0);
      if (vals.length < 2) continue;
      b[c.key] = c.better === "max" ? Math.max(...vals) : Math.min(...vals);
    }
    return b;
  }, [nodes]);

  const charts = useMemo(() => {
    let longest: number[] = [];
    for (const n of nodes) {
      const s = metrics[n.agentId] ?? [];
      if (s.length > longest.length) longest = s.map((v) => v.t);
    }
    const x = secondsFromStart(longest);
    const mk = (pick: (v: MetricSample) => number) =>
      nodes.map((n, i) => ({
        label: n.hostname,
        data: (metrics[n.agentId] ?? []).map(pick),
        color: HOST_COLORS[i % HOST_COLORS.length],
      }));
    // Fault markers are positioned relative to the first sample timestamp so they
    // line up with the seconds-from-start x-axis.
    const t0 = longest.length ? longest[0] : 0;
    const markers: ChartMarker[] = [];
    if (t0 > 0) {
      for (const f of run?.faults ?? []) {
        markers.push({
          x: Math.max(0, Math.round((f.tsInjectMilli - t0) / 1000)),
          label: f.detail || f.kind,
          kind: "inject",
        });
        if (f.tsRevertMilli > 0) {
          markers.push({
            x: Math.max(0, Math.round((f.tsRevertMilli - t0) / 1000)),
            label: "Ende: " + (f.detail || f.kind),
            kind: "revert",
          });
        }
      }
    }
    return {
      x,
      markers,
      iops: mk((v) => (v.ior ?? 0) + (v.iow ?? 0)),
      lat: mk((v) => v.l99 ?? 0),
    };
  }, [nodes, metrics, run]);

  if (err && !run)
    return (
      <div className="callout err">
        <Icon name="alert" />
        <div>
          <div className="title">Lauf konnte nicht geladen werden</div>
          <div className="body">{err}</div>
        </div>
      </div>
    );
  if (!run) return <div className="panel empty">Lädt…</div>;

  const active = ACTIVE_RUN.has(run.status);
  const problems = nodes.filter((n) => (n.phase === "failed" || n.phase === "aborted") && n.phaseMessage);
  const specs = Object.entries(run.resolvedConfig ?? {});
  const sameSpec = specs.length > 1 && specs.every(([, s]) => JSON.stringify(s) === JSON.stringify(specs[0][1]));

  return (
    <div className="stack">
      <PageHeader
        crumbs={[{ to: "/results?tab=runs", label: "Ergebnisse" }]}
        title={
          <>
            {run.name || "Lauf"} <span className="mono faint" style={{ fontSize: 14, fontWeight: 500 }}>{shortId(run.id)}</span>
          </>
        }
        meta={
          <>
            <RunStatusBadge status={run.status} />
            <span>{fmtDateTime(run.startedAt)}</span>
            <span className="sep">|</span>
            <span className="row" style={{ gap: 5 }}>
              <Icon name="clock" size={14} />
              {fmtDuration(elapsedSec(run.startedAt, run.endedAt, active))}
            </span>
            <span className="sep">|</span>
            <span>
              {nodes.length} {nodes.length === 1 ? "Host" : "Hosts"}
            </span>
          </>
        }
        actions={
          active ? (
            <>
              <button
                className="btn secondary"
                disabled={busy}
                onClick={() => doStop(false)}
                title="Beendet fio sauber, Ergebnisse bis hierhin bleiben erhalten"
              >
                <Icon name="stop" size={12} /> Stoppen
              </button>
              <button className="btn danger-ghost" disabled={busy} onClick={() => setConfirmAbort(true)}>
                Sofort abbrechen
              </button>
            </>
          ) : (
            run.resolvedConfig && (
              <button className="btn secondary" disabled={busy} onClick={doRestart} title="Startet denselben Lauf mit identischer Konfiguration">
                <Icon name="restart" /> Erneut ausführen
              </button>
            )
          )
        }
      />

      {err && (
        <div className="callout err">
          <Icon name="alert" />
          <div className="body">{err}</div>
        </div>
      )}

      {problems.length > 0 && (
        <div className="callout err">
          <Icon name="alert" />
          <div>
            <div className="title">
              {problems.length === 1 ? "Ein Host hat" : `${problems.length} Hosts haben`} den Lauf nicht abgeschlossen
            </div>
            {problems.map((n) => (
              <div key={n.agentId} className="body">
                <b style={{ color: "var(--text)" }}>{n.hostname}:</b> <span className="mono">{n.phaseMessage}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Results per host */}
      <section className="panel">
        <div className="panel-head">
          <h2>Ergebnis je Host</h2>
          {nodes.length > 1 && (
            <span className="sub">
              <span style={{ color: "var(--ok)", fontWeight: 600 }}>Grün</span> = bester Wert der Spalte
            </span>
          )}
        </div>
        {nodes.length === 0 ? (
          <Empty title="Keine Hosts in diesem Lauf" compact />
        ) : (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th className="group" colSpan={2} />
                  <th className="group num col-sep" colSpan={2}>IOPS</th>
                  <th className="group num col-sep" colSpan={2}>Durchsatz</th>
                  <th className="group num col-sep" colSpan={4}>Latenz</th>
                  <th className="group num col-sep" />
                </tr>
                <tr>
                  <th>Host</th>
                  <th>Status</th>
                  {COLS.map((c) => (
                    <th key={c.key} className={`num ${c.sep ? "col-sep" : ""}`}>
                      {c.label}
                    </th>
                  ))}
                  <th className="num col-sep" title="Schwankung der IOPS über die Zeit (Variationskoeffizient). Kleiner = stabiler.">
                    Schwankung
                  </th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => {
                  const f = n.fioSummary ?? undefined;
                  const s = metrics[n.agentId] ?? [];
                  const cv = coefficientOfVariation(s.map((v) => (v.ior ?? 0) + (v.iow ?? 0)));
                  return (
                    <tr key={n.agentId}>
                      <td className="cell-title">{n.hostname}</td>
                      <td>
                        <RunStatusBadge status={n.phase} />
                      </td>
                      {COLS.map((c) => {
                        const v = f?.[c.key];
                        const isBest = v != null && best[c.key] === v;
                        return (
                          <td key={c.key} className={`num ${c.sep ? "col-sep" : ""} ${isBest ? "best" : ""}`}>
                            {f ? c.fmt(v) : <span className="faint">–</span>}
                          </td>
                        );
                      })}
                      <td className="num col-sep">
                        {cv == null ? (
                          <span className="faint">–</span>
                        ) : (
                          <span
                            className={`badge ${cv < 0.1 ? "ok" : cv < 0.25 ? "warn" : "err"}`}
                            title={cv < 0.1 ? "stabil" : cv < 0.25 ? "schwankend" : "stark schwankend"}
                          >
                            ±{fmtPct(cv)}
                          </span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Time series */}
      <div className="split" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(420px, 1fr))" }}>
        <section className="panel">
          <div className="panel-body">
            <Chart title="IOPS gesamt" x={charts.x} height={230} yFormat={fmtIOPS} series={charts.iops} fill={nodes.length === 1} markers={charts.markers} />
          </div>
        </section>
        <section className="panel">
          <div className="panel-body">
            <Chart title="Latenz p99" x={charts.x} height={230} yFormat={fmtLatUs} series={charts.lat} fill={false} markers={charts.markers} />
          </div>
        </section>
      </div>

      {/* Fault injection (resilience testing) */}
      <FaultPanel runId={run.id} nodes={nodes} agents={agents} faults={run.faults ?? []} onChanged={load} />

      <div className="split">
        {/* Events */}
        <section className="panel">
          <div className="panel-head">
            <h2>Ereignisse</h2>
            <span className="sub">{events.length}</span>
          </div>
          {events.length === 0 ? (
            <Empty title="Keine Ereignisse" compact />
          ) : (
            <div className="table-wrap" style={{ maxHeight: 420, overflowY: "auto" }}>
              <table className="data">
                <thead>
                  <tr>
                    <th>Zeit</th>
                    <th>Host</th>
                    <th>Art</th>
                    <th>Meldung</th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((e, i) => (
                    <tr key={i}>
                      <td className="mono muted">{new Date(e.tsWallMilli).toLocaleTimeString("de-DE")}</td>
                      <td>{hostName[e.agentId] ?? e.agentId}</td>
                      <td>
                        <span className={`badge ${e.severity === "error" ? "err" : e.severity === "warn" ? "warn" : ""}`}>
                          {e.severity === "error" ? "Fehler" : e.severity === "warn" ? "Warnung" : "Info"}
                        </span>
                      </td>
                      <td>
                        {e.message}
                        <span className="faint" style={{ marginLeft: 8, fontSize: 12 }}>
                          {e.source}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        {/* Configuration used */}
        <section className="panel">
          <div className="panel-head">
            <h2>Konfiguration</h2>
            <span className="sub">{sameSpec || specs.length <= 1 ? "auf allen Hosts gleich" : "je Host"}</span>
          </div>
          <div className="panel-body stack" style={{ gap: 16 }}>
            {specs.length === 0 && <span className="faint">Keine Konfiguration gespeichert.</span>}
            {(sameSpec ? specs.slice(0, 1) : specs).map(([agentId, s]) => (
              <div key={agentId}>
                {!sameSpec && specs.length > 1 && (
                  <div className="field-label" style={{ marginBottom: 6 }}>
                    {hostName[agentId] ?? agentId}
                  </div>
                )}
                <SpecList spec={s} />
              </div>
            ))}
          </div>
        </section>
      </div>

      {confirmAbort && (
        <ConfirmDialog
          title="Lauf sofort abbrechen?"
          confirmLabel="Sofort abbrechen"
          danger
          busy={busy}
          onCancel={() => setConfirmAbort(false)}
          onConfirm={() => doStop(true)}
        >
          fio wird auf allen Hosts hart beendet, ein Ergebnis für diesen Lauf ist dann möglicherweise nicht
          verfügbar. Zum sauberen Beenden „Stoppen“ verwenden.
        </ConfirmDialog>
      )}
    </div>
  );
}

function SpecList({ spec: s }: { spec: Spec }) {
  return (
    <dl className="dl">
      <dt>Workload</dt>
      <dd>{workloadLabel(s)}</dd>
      <dt>Blockgröße</dt>
      <dd>{s.blockSize}</dd>
      <dt>Parallelität</dt>
      <dd>
        QD {s.ioDepth} × {s.numJobs} Jobs · {s.engine}
      </dd>
      <dt>Direct I/O</dt>
      <dd>{s.directIO ? "ja" : "nein"}</dd>
      <dt>Dauer</dt>
      <dd>
        {s.runtimeSeconds} s{s.rampSeconds ? ` + ${s.rampSeconds} s Warmlauf` : ""}
      </dd>
      <dt>Ziel</dt>
      <dd>
        {s.targetPath}
        {s.targetKind === "file" && s.fileSizeMiB ? ` (${s.fileSizeMiB} MiB)` : ""}
      </dd>
      {!!s.targetIOPS && (
        <>
          <dt>IOPS-Limit</dt>
          <dd>{s.targetIOPS}</dd>
        </>
      )}
    </dl>
  );
}
