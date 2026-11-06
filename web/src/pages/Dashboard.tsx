import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useLive } from "../state/LiveProvider";
import { api } from "../api/client";
import type { AgentView, Batch, MetricSample, Profile, RunSummary } from "../api/types";
import { Sparkline } from "../components/Sparkline";
import { RunStatusBadge, StatusBadge } from "../components/StatusBadge";
import { Empty, PageHeader, TimeCell } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { RunProfileDialog } from "../components/RunProfileDialog";
import { ACTIVE_PHASES, ACTIVE_RUN, profileSeconds, statusInfo } from "../domain";
import { elapsedSec, fmtDuration, fmtIOPS, fmtLatUs, fmtRelative, parseTime, shortId } from "../format";

const DAY = 24 * 3600 * 1000;

export function Dashboard() {
  const nav = useNavigate();
  const { agents, liveMetrics, testStates } = useLive();
  const [runs, setRuns] = useState<RunSummary[] | null>(null);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [profiles, setProfiles] = useState<Profile[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [fire, setFire] = useState<Profile | null>(null);

  useEffect(() => {
    let active = true;
    const load = () => {
      api
        .runs()
        .then((r) => {
          if (!active) return;
          setRuns(r ?? []);
          setErr(null);
        })
        .catch((e) => active && setErr(e instanceof Error ? e.message : String(e)));
      api.batches().then((b) => active && setBatches(b ?? [])).catch(() => {});
      api.profiles().then((p) => active && setProfiles(p ?? [])).catch(() => active && setProfiles([]));
    };
    load();
    const t = setInterval(load, 8000);
    return () => {
      active = false;
      clearInterval(t);
    };
  }, []);

  const online = agents.filter((a) => a.status === "online").length;
  const measuring = agents.filter((a) => ACTIVE_PHASES.has(testStates[a.agentId]?.phase ?? ""));
  const activeBatches = batches.filter((b) => ACTIVE_RUN.has(b.status));
  const recent = (runs ?? []).filter((r) => (parseTime(r.startedAt) ?? 0) > Date.now() - DAY);
  const failed24 = recent.filter((r) => r.status === "failed").length;
  const last = runs?.[0];
  const lastInfo = statusInfo(last?.status);

  return (
    <div className="stack">
      <PageHeader
        title="Übersicht"
        meta={<span>Zustand der Testhosts und letzte Messungen</span>}
        actions={
          <>
            <Link className="btn secondary" to="/profiles/new">
              <Icon name="plus" />
              Neues Profil
            </Link>
            <Link className="btn" to="/profiles">
              <Icon name="play" size={14} />
              Profil ausführen
            </Link>
          </>
        }
      />

      {/* Key figures — one surface, four cells */}
      <section className="panel kpis" aria-label="Kennzahlen">
        <div className="kpi">
          <div className="kpi-label">Hosts online</div>
          <div className="kpi-value">
            {online}
            <span className="of"> / {agents.length}</span>
          </div>
          <div className={`kpi-sub ${online < agents.length ? "err" : ""}`}>
            {agents.length === 0
              ? "kein Agent verbunden"
              : online === agents.length
                ? "alle erreichbar"
                : `${agents.length - online} nicht erreichbar`}
          </div>
        </div>
        <div className="kpi">
          <div className="kpi-label">Messung läuft</div>
          <div className="kpi-value">{measuring.length}</div>
          <div className="kpi-sub">
            {activeBatches.length > 0
              ? `${activeBatches[0].name} · Schritt ${Math.min(activeBatches[0].stepIndex + 1, activeBatches[0].totalSteps)}/${activeBatches[0].totalSteps}`
              : "keine aktive Messung"}
          </div>
        </div>
        <div className="kpi">
          <div className="kpi-label">Läufe in 24 h</div>
          <div className="kpi-value">{runs == null ? "–" : recent.length}</div>
          <div className={`kpi-sub ${failed24 > 0 ? "err" : ""}`}>
            {recent.length === 0
              ? "keine Läufe"
              : failed24 > 0
                ? `${failed24} fehlgeschlagen`
                : "alle ohne Fehler"}
          </div>
        </div>
        <div className="kpi">
          <div className="kpi-label">Letzter Lauf</div>
          <div
            className="kpi-value"
            style={{
              fontFamily: "var(--font)",
              fontSize: 19,
              paddingTop: 3,
              color: lastInfo.tone ? `var(--${lastInfo.tone === "accent" ? "accent-text" : lastInfo.tone})` : undefined,
            }}
          >
            {last ? lastInfo.label : "–"}
          </div>
          <div className="kpi-sub">{last ? `${last.name || shortId(last.id)} · ${fmtRelative(last.startedAt)}` : "noch nichts gemessen"}</div>
        </div>
      </section>

      {/* Running profile runs */}
      {activeBatches.map((b) => (
        <Link key={b.id} to={`/batches/${b.id}`} className="panel" style={{ display: "block" }}>
          <div className="panel-head" style={{ borderBottom: "none" }}>
            <RunStatusBadge status={b.status} />
            <h2>{b.name}</h2>
            <span className="sub">
              Schritt {Math.min(b.stepIndex + 1, b.totalSteps)} von {b.totalSteps} · {b.agentIds.length}{" "}
              {b.agentIds.length === 1 ? "Host" : "Hosts"}
            </span>
            <span className="actions muted row" style={{ gap: 6 }}>
              Verlauf ansehen <Icon name="arrow" size={14} />
            </span>
          </div>
        </Link>
      ))}

      {/* Hosts */}
      <section className="panel">
        <div className="panel-head">
          <h2>Hosts</h2>
          <span className="sub">{agents.length} registriert</span>
          <div className="actions">
            <Link className="btn ghost sm" to="/live">
              Live-Ansicht <Icon name="arrow" size={14} />
            </Link>
          </div>
        </div>
        {agents.length === 0 ? (
          <Empty title="Noch kein Host verbunden">
            Installiere den Agenten auf einem Testserver (deploy/agent/install-agent.sh). Er meldet sich
            hier automatisch an.
          </Empty>
        ) : (
          <div className="hosts">
            {agents.map((a) => (
              <HostTile
                key={a.agentId}
                agent={a}
                samples={liveMetrics[a.agentId] ?? []}
                phase={testStates[a.agentId]?.phase}
              />
            ))}
          </div>
        )}
      </section>

      <div className="split">
        {/* Recent runs */}
        <section className="panel">
          <div className="panel-head">
            <h2>Letzte Läufe</h2>
            <div className="actions">
              <Link className="btn ghost sm" to="/results">
                Alle Ergebnisse <Icon name="arrow" size={14} />
              </Link>
            </div>
          </div>
          {err && (
            <div className="callout err" style={{ margin: 16 }}>
              <Icon name="alert" />
              <div>
                <div className="title">Läufe konnten nicht geladen werden</div>
                <div className="body">{err}</div>
              </div>
            </div>
          )}
          {runs != null && runs.length === 0 ? (
            <Empty title="Noch keine Läufe" compact>
              Führe ein Profil aus, um die erste Messung zu erzeugen.
            </Empty>
          ) : (
            <div className="table-wrap">
              <table className="data">
                <thead>
                  <tr>
                    <th>Lauf</th>
                    <th>Status</th>
                    <th>Gestartet</th>
                    <th className="num">Dauer</th>
                    <th aria-label="Öffnen" />
                  </tr>
                </thead>
                <tbody>
                  {(runs ?? []).slice(0, 8).map((r) => (
                    <tr key={r.id} className="click" onClick={() => nav(`/runs/${r.id}`)}>
                      <td>
                        <Link to={`/runs/${r.id}`} className="cell-title" onClick={(e) => e.stopPropagation()}>
                          {r.name || "Lauf"}
                        </Link>
                        <span className="mono faint" style={{ marginLeft: 8 }}>
                          {shortId(r.id)}
                        </span>
                      </td>
                      <td>
                        <RunStatusBadge status={r.status} />
                      </td>
                      <td>
                        <TimeCell iso={r.startedAt} />
                      </td>
                      <td className="num">
                        {fmtDuration(elapsedSec(r.startedAt, r.endedAt, ACTIVE_RUN.has(r.status)))}
                      </td>
                      <td className="go">
                        <Icon name="right" size={14} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        {/* Quick start */}
        <section className="panel">
          <div className="panel-head">
            <h2>Schnellstart</h2>
            <span className="sub">Profil direkt ausführen</span>
          </div>
          {profiles != null && profiles.length === 0 ? (
            <Empty
              title="Noch kein Profil"
              compact
              action={
                <Link className="btn sm" to="/profiles/new">
                  <Icon name="plus" /> Profil anlegen
                </Link>
              }
            >
              Ein Profil beschreibt eine Folge von fio-Messungen.
            </Empty>
          ) : (
            <div className="table-wrap">
              <table className="data">
                <tbody>
                  {(profiles ?? []).slice(0, 6).map((p) => (
                    <tr key={p.id}>
                      <td>
                        <Link to={`/profiles/${p.id}`} className="cell-title">
                          {p.name}
                        </Link>
                        <div className="cell-sub">
                          {p.steps.length} {p.steps.length === 1 ? "Schritt" : "Schritte"} · ca.{" "}
                          {fmtDuration(profileSeconds(p.steps))}
                        </div>
                      </td>
                      <td style={{ textAlign: "right", width: 1 }}>
                        <button
                          className="btn secondary sm"
                          onClick={() => setFire(p)}
                          disabled={online === 0}
                          title={online === 0 ? "Kein Host online" : "Profil ausführen"}
                        >
                          <Icon name="play" size={12} />
                          Ausführen
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>

      {fire && <RunProfileDialog profile={fire} onClose={() => setFire(null)} />}
    </div>
  );
}

function HostTile({
  agent: a,
  samples,
  phase,
}: {
  agent: AgentView;
  samples: MetricSample[];
  phase?: string;
}) {
  const active = ACTIVE_PHASES.has(phase ?? "");
  const lastS = samples[samples.length - 1];
  const caps = a.capabilities;
  const mem = a.system.memTotalMB ? `${Math.round(a.system.memTotalMB / 1024)} GiB RAM` : null;
  const cpu = a.system.cpuCores ? `${a.system.cpuCores} Kerne` : null;

  return (
    <div className="host">
      <div className="row" style={{ alignItems: "flex-start" }}>
        <div className="grow">
          <div className="host-name ellipsis">{a.system.hostname}</div>
          <div className="host-meta">
            {[a.system.distro ?? a.system.os, a.system.arch, cpu, mem].filter(Boolean).join(" · ")}
          </div>
        </div>
        <StatusBadge status={a.status} />
      </div>
      <div className="host-tags">
        {caps.fio ? (
          <span className="tag">fio {caps.fioVersion ?? ""}</span>
        ) : (
          <span className="badge err">fio fehlt</span>
        )}
        {caps.ioUring && <span className="tag">io_uring</span>}
        {caps.libaio && <span className="tag">libaio</span>}
        {caps.nvme && <span className="tag">nvme</span>}
        {caps.multipath && <span className="tag">multipath</span>}
      </div>
      <div className="host-activity">
        {active ? (
          <>
            <div>
              <div className="readout-label">IOPS</div>
              <div className="readout-value">{fmtIOPS((lastS?.ior ?? 0) + (lastS?.iow ?? 0))}</div>
            </div>
            <div>
              <div className="readout-label">Latenz p99</div>
              <div className="readout-value">{fmtLatUs(lastS?.l99)}</div>
            </div>
            <div className="grow">
              <Sparkline values={samples.slice(-60).map((s) => (s.ior ?? 0) + (s.iow ?? 0))} />
            </div>
          </>
        ) : (
          <>
            <span className="muted">
              {a.status === "offline" ? "Nicht erreichbar" : phase && phase !== "idle" ? `Letzte Messung: ${statusInfo(phase).label}` : "Bereit für Messungen"}
            </span>
            <span className="grow" />
            {a.status !== "offline" && (
              <span className="faint mono" title="Round-Trip-Zeit zum Controller">
                RTT {a.rttMs} ms
              </span>
            )}
          </>
        )}
      </div>
    </div>
  );
}
