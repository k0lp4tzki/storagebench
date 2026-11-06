import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useLive } from "../state/LiveProvider";
import { api } from "../api/client";
import type { AgentView, MetricSample } from "../api/types";
import { Chart } from "../components/Chart";
import { RunStatusBadge, StatusBadge } from "../components/StatusBadge";
import { Empty, PageHeader } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { ACTIVE_PHASES } from "../domain";
import { fmtBW, fmtIOPS, fmtLatUs, secondsFromStart, shortId } from "../format";

const hasIO = (s: MetricSample[]) => s.some((v) => (v.ior ?? 0) > 0 || (v.iow ?? 0) > 0 || (v.l99 ?? 0) > 0);

export function LiveView() {
  const { agents, liveMetrics, testStates } = useLive();
  const [stopping, setStopping] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const withData = agents.filter(
    (a) => ACTIVE_PHASES.has(testStates[a.agentId]?.phase ?? "") || hasIO(liveMetrics[a.agentId] ?? []),
  );
  const without = agents.filter((a) => !withData.includes(a));
  const activeCount = agents.filter((a) => ACTIVE_PHASES.has(testStates[a.agentId]?.phase ?? "")).length;
  const activeRuns = [
    ...new Set(
      agents
        .filter((a) => ACTIVE_PHASES.has(testStates[a.agentId]?.phase ?? ""))
        .map((a) => testStates[a.agentId]!.runId),
    ),
  ];

  async function stop(runId: string) {
    setStopping(runId);
    setErr(null);
    try {
      await api.stopRun(runId);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setStopping(null);
    }
  }

  return (
    <div className="stack">
      <PageHeader
        title="Live"
        meta={
          activeCount > 0 ? (
            <>
              <span className="badge accent">
                <span className="dot" />
                Messung läuft
              </span>
              <span>
                {activeCount} von {agents.length} Hosts messen gerade
              </span>
            </>
          ) : (
            <span>Echtzeit-Messwerte der Hosts, aktualisiert jede Sekunde</span>
          )
        }
        actions={activeRuns.map((id) => (
          <span key={id} className="row" style={{ gap: 8 }}>
            <Link className="btn secondary" to={`/runs/${id}`}>
              Lauf {shortId(id)} öffnen
            </Link>
            <button
              className="btn danger-ghost"
              onClick={() => stop(id)}
              disabled={stopping === id}
              title="Beendet den Lauf sauber, bisherige Ergebnisse bleiben erhalten"
            >
              <Icon name="stop" size={12} />
              {stopping === id ? "Stoppt…" : "Stoppen"}
            </button>
          </span>
        ))}
      />

      {err && (
        <div className="callout err">
          <Icon name="alert" />
          <div>
            <div className="title">Stoppen fehlgeschlagen</div>
            <div className="body">{err}</div>
          </div>
        </div>
      )}

      {agents.length === 0 && (
        <div className="panel">
          <Empty title="Keine Hosts verbunden">Sobald ein Agent verbunden ist, erscheinen hier seine Messwerte.</Empty>
        </div>
      )}

      {agents.length > 0 && withData.length === 0 && (
        <div className="panel">
          <Empty
            title="Gerade läuft keine Messung"
            action={
              <Link className="btn" to="/profiles">
                <Icon name="play" size={14} /> Profil ausführen
              </Link>
            }
          >
            Starte ein Profil. IOPS, Durchsatz und Latenz jedes Hosts erscheinen dann hier in Echtzeit.
          </Empty>
        </div>
      )}

      {withData.map((a) => (
        <LiveHost
          key={a.agentId}
          agent={a}
          samples={liveMetrics[a.agentId] ?? []}
          state={testStates[a.agentId]}
        />
      ))}

      {without.length > 0 && (
        <section className="panel">
          <div className="panel-head">
            <h2>{withData.length > 0 ? "Weitere Hosts" : "Hosts"}</h2>
            <span className="sub">ohne laufende Messung</span>
          </div>
          <div className="table-wrap">
            <table className="data">
              <tbody>
                {without.map((a) => (
                  <tr key={a.agentId}>
                    <td className="cell-title">{a.system.hostname}</td>
                    <td className="muted">{a.system.distro ?? a.system.os}</td>
                    <td>
                      <StatusBadge status={a.status} />
                    </td>
                    <td className="muted" style={{ textAlign: "right" }}>
                      {a.status === "offline" ? "nicht erreichbar" : "bereit"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}

function LiveHost({
  agent: a,
  samples,
  state,
}: {
  agent: AgentView;
  samples: MetricSample[];
  state?: { runId: string; phase: string };
}) {
  const last = samples[samples.length - 1];
  const x = useMemo(() => secondsFromStart(samples.map((s) => s.t)), [samples]);
  const iops = useMemo(
    () => [
      { label: "Read", data: samples.map((s) => s.ior ?? 0), color: "var(--read)" },
      { label: "Write", data: samples.map((s) => s.iow ?? 0), color: "var(--write)" },
    ],
    [samples],
  );
  const lat = useMemo(
    () => [
      { label: "Ø", data: samples.map((s) => s.lm ?? 0), color: "var(--lat-mean)" },
      { label: "p99", data: samples.map((s) => s.l99 ?? 0), color: "var(--lat-tail)" },
    ],
    [samples],
  );
  const active = ACTIVE_PHASES.has(state?.phase ?? "");

  return (
    <section className="panel">
      <div className="panel-head">
        <h2>{a.system.hostname}</h2>
        {state ? <RunStatusBadge status={state.phase} /> : <StatusBadge status={a.status} />}
        {!active && <span className="sub">zeigt die zuletzt empfangenen Werte</span>}
        {state?.runId && (
          <div className="actions">
            <Link className="btn ghost sm" to={`/runs/${state.runId}`}>
              Lauf <span className="mono">{shortId(state.runId)}</span> <Icon name="arrow" size={14} />
            </Link>
          </div>
        )}
      </div>
      <div className="live-host">
        <div className="live-side">
          <div className="readout-label">IOPS gesamt</div>
          <div className="readout-value" style={{ fontSize: 26 }}>
            {fmtIOPS((last?.ior ?? 0) + (last?.iow ?? 0))}
          </div>
          <div className="readouts">
            <Readout label="Read" value={fmtIOPS(last?.ior)} color="var(--read)" />
            <Readout label="Write" value={fmtIOPS(last?.iow)} color="var(--write)" />
            <Readout label="Durchsatz R" value={fmtBW(last?.bwr)} />
            <Readout label="Durchsatz W" value={fmtBW(last?.bww)} />
            <Readout label="Latenz Ø" value={fmtLatUs(last?.lm)} />
            <Readout label="Latenz p99" value={fmtLatUs(last?.l99)} />
          </div>
        </div>
        <div className="live-chart">
          <Chart title="IOPS" x={x} height={190} yFormat={fmtIOPS} series={iops} />
        </div>
        <div className="live-chart">
          <Chart title="Latenz" x={x} height={190} yFormat={fmtLatUs} series={lat} fill={false} />
        </div>
      </div>
    </section>
  );
}

function Readout({ label, value, color }: { label: string; value: string; color?: string }) {
  return (
    <div>
      <div className="readout-label row" style={{ gap: 6 }}>
        {color && <i style={{ width: 8, height: 3, borderRadius: 2, background: color, display: "inline-block" }} />}
        {label}
      </div>
      <div className="readout-value" style={{ fontSize: 14 }}>
        {value}
      </div>
    </div>
  );
}
