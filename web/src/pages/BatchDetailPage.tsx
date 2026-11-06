import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../api/client";
import { useLive } from "../state/LiveProvider";
import type { BatchDetail } from "../api/types";
import { RunStatusBadge } from "../components/StatusBadge";
import { Empty, PageHeader, TimeCell } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { ACTIVE_RUN, statusInfo } from "../domain";
import { elapsedSec, fmtDateTime, fmtDuration, shortId } from "../format";

export function BatchDetailPage() {
  const { id = "" } = useParams();
  const nav = useNavigate();
  const { batchProgress } = useLive();
  const [batch, setBatch] = useState<BatchDetail | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setBatch(await api.batch(id));
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const live = batchProgress[id];
  useEffect(() => {
    if (live) load();
  }, [live, load]);

  useEffect(() => {
    if (!batch || !ACTIVE_RUN.has(batch.status)) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [batch, load]);

  if (err && !batch)
    return (
      <div className="callout err">
        <Icon name="alert" />
        <div>
          <div className="title">Profillauf konnte nicht geladen werden</div>
          <div className="body">{err}</div>
        </div>
      </div>
    );
  if (!batch) return <div className="panel empty">Lädt…</div>;

  const runs = batch.runs ?? [];
  const active = ACTIVE_RUN.has(batch.status);
  const byStep = new Map(runs.map((r) => [r.step, r]));
  const current = Math.min(batch.stepIndex + 1, batch.totalSteps);

  return (
    <div className="stack">
      <PageHeader
        crumbs={[{ to: "/results", label: "Ergebnisse" }]}
        title={
          <>
            {batch.name || "Profillauf"}{" "}
            <span className="mono faint" style={{ fontSize: 14, fontWeight: 500 }}>
              {shortId(batch.id)}
            </span>
          </>
        }
        meta={
          <>
            <RunStatusBadge status={batch.status} />
            {batch.profileName && (
              <>
                <span>
                  Profil{" "}
                  {batch.profileId ? (
                    <Link to={`/profiles/${batch.profileId}`} style={{ color: "var(--accent-text)" }}>
                      {batch.profileName}
                    </Link>
                  ) : (
                    batch.profileName
                  )}
                </span>
                <span className="sep">|</span>
              </>
            )}
            <span>{fmtDateTime(batch.startedAt)}</span>
            <span className="sep">|</span>
            <span className="row" style={{ gap: 5 }}>
              <Icon name="clock" size={14} />
              {fmtDuration(elapsedSec(batch.startedAt, batch.endedAt, active))}
            </span>
            <span className="sep">|</span>
            <span>
              {batch.agentIds.length} {batch.agentIds.length === 1 ? "Host" : "Hosts"}
            </span>
          </>
        }
        actions={
          active && (
            <Link className="btn secondary" to="/live">
              <Icon name="live" /> Live verfolgen
            </Link>
          )
        }
      />

      <section className="panel">
        <div className="panel-head">
          <h2>Ablauf</h2>
          <span className="sub">
            {batch.status === "completed"
              ? `alle ${batch.totalSteps} Schritte abgeschlossen`
              : active
                ? `Schritt ${current} von ${batch.totalSteps} läuft`
                : `angehalten bei Schritt ${current} von ${batch.totalSteps}`}
          </span>
        </div>
        <div style={{ padding: "14px 16px 4px" }}>
          <div className="track" aria-hidden>
            {Array.from({ length: batch.totalSteps }, (_, i) => {
              const r = byStep.get(i);
              return <span key={i} className={r ? statusInfo(r.status).tone : ""} title={`Schritt ${i + 1}`} />;
            })}
          </div>
        </div>
        {runs.length === 0 ? (
          <Empty title="Noch kein Schritt gestartet" compact />
        ) : (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th className="num" style={{ width: 40 }}>#</th>
                  <th>Schritt</th>
                  <th>Status</th>
                  <th>Gestartet</th>
                  <th className="num">Dauer</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {runs.map((r) => (
                  <tr key={r.runId} className="click" onClick={() => nav(`/runs/${r.runId}`)}>
                    <td className="num muted">{r.step + 1}</td>
                    <td className="cell-title">{r.name}</td>
                    <td>
                      <RunStatusBadge status={r.status} />
                    </td>
                    <td>
                      <TimeCell iso={r.startedAt} />
                    </td>
                    <td className="num">{fmtDuration(elapsedSec(r.startedAt, r.endedAt, ACTIVE_RUN.has(r.status)))}</td>
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
    </div>
  );
}
