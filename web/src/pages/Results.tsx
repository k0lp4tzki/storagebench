import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { Batch, RunSummary } from "../api/types";
import { RunStatusBadge } from "../components/StatusBadge";
import { Empty, PageHeader, TimeCell } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { ACTIVE_RUN, bucketOf, type Bucket } from "../domain";
import { elapsedSec, fmtDuration, shortId } from "../format";

type Tab = "batches" | "runs";

const BUCKETS: { id: Bucket; label: string }[] = [
  { id: "all", label: "Alle" },
  { id: "ok", label: "Abgeschlossen" },
  { id: "failed", label: "Fehlgeschlagen" },
  { id: "aborted", label: "Abgebrochen" },
  { id: "active", label: "Laufend" },
];

export function Results() {
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const tab = (params.get("tab") as Tab) || "batches";
  const bucket = (params.get("status") as Bucket) || "all";
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [q, setQ] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const setParam = (k: string, v: string, def: string) => {
    const next = new URLSearchParams(params);
    if (v === def) next.delete(k);
    else next.set(k, v);
    setParams(next, { replace: true });
  };

  useEffect(() => {
    Promise.all([api.runs(), api.batches()])
      .then(([r, b]) => {
        setRuns(r ?? []);
        setBatches(b ?? []);
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, []);

  const items: (RunSummary | Batch)[] = tab === "runs" ? runs : batches;
  const counts = useMemo(() => {
    const c: Record<Bucket, number> = { all: items.length, ok: 0, failed: 0, aborted: 0, active: 0 };
    for (const it of items) {
      const b = bucketOf(it.status);
      if (b !== "all") c[b]++;
    }
    return c;
  }, [items]);

  const match = (name: string, id: string, status: string) =>
    (bucket === "all" || bucketOf(status) === bucket) &&
    (name + " " + id).toLowerCase().includes(q.trim().toLowerCase());

  const shownRuns = runs.filter((r) => match(r.name, r.id, r.status));
  const shownBatches = batches.filter((b) => match(b.name, b.id, b.status));
  const shownCount = tab === "runs" ? shownRuns.length : shownBatches.length;

  return (
    <div className="stack">
      <PageHeader
        title="Ergebnisse"
        meta={<span>Alle Profilläufe und die einzelnen Messläufe, neueste zuerst</span>}
      />

      {err && (
        <div className="callout err">
          <Icon name="alert" />
          <div>
            <div className="title">Ergebnisse konnten nicht geladen werden</div>
            <div className="body">{err}</div>
          </div>
        </div>
      )}

      <section className="panel">
        <div className="panel-head" style={{ flexWrap: "wrap", rowGap: 10 }}>
          <div className="seg" role="tablist">
            <button role="tab" aria-selected={tab === "batches"} className={tab === "batches" ? "on" : ""} onClick={() => setParam("tab", "batches", "batches")}>
              Profilläufe <span className="n">{batches.length}</span>
            </button>
            <button role="tab" aria-selected={tab === "runs"} className={tab === "runs" ? "on" : ""} onClick={() => setParam("tab", "runs", "batches")}>
              Einzelne Läufe <span className="n">{runs.length}</span>
            </button>
          </div>
          <div className="actions" style={{ flexWrap: "wrap" }}>
            <div className="seg" aria-label="Status filtern">
              {BUCKETS.map((b) => (
                <button key={b.id} className={bucket === b.id ? "on" : ""} onClick={() => setParam("status", b.id, "all")}>
                  {b.label}
                  {b.id !== "all" && counts[b.id] > 0 && <span className="n">{counts[b.id]}</span>}
                </button>
              ))}
            </div>
            <div className="search">
              <Icon name="search" size={14} />
              <input placeholder="Name oder ID…" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
          </div>
        </div>

        {loading ? (
          <div className="empty">Lädt…</div>
        ) : shownCount === 0 ? (
          <Empty title={items.length === 0 ? "Noch keine Ergebnisse" : "Keine Treffer"} compact>
            {items.length === 0 ? (
              <>
                Führe ein <Link to="/profiles" style={{ color: "var(--accent-text)" }}>Profil</Link> aus, um Messungen zu erzeugen.
              </>
            ) : (
              "Passe Filter oder Suche an."
            )}
          </Empty>
        ) : tab === "batches" ? (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th>Profillauf</th>
                  <th>Status</th>
                  <th>Fortschritt</th>
                  <th className="num">Hosts</th>
                  <th>Gestartet</th>
                  <th className="num">Dauer</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {shownBatches.map((b) => {
                  const at = b.status === "completed" ? b.totalSteps : Math.min(b.stepIndex + 1, b.totalSteps);
                  return (
                    <tr key={b.id} className="click" onClick={() => nav(`/batches/${b.id}`)}>
                      <td>
                        <Link to={`/batches/${b.id}`} className="cell-title" onClick={(e) => e.stopPropagation()}>
                          {b.name || "Profillauf"}
                        </Link>
                        <span className="mono faint" style={{ marginLeft: 8 }}>{shortId(b.id)}</span>
                      </td>
                      <td>
                        <RunStatusBadge status={b.status} />
                      </td>
                      <td className="mono muted">
                        Schritt {at} / {b.totalSteps}
                      </td>
                      <td className="num">{b.agentIds.length}</td>
                      <td>
                        <TimeCell iso={b.startedAt} />
                      </td>
                      <td className="num">{fmtDuration(elapsedSec(b.startedAt, b.endedAt, ACTIVE_RUN.has(b.status)))}</td>
                      <td className="go">
                        <Icon name="right" size={14} />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th>Lauf</th>
                  <th>Status</th>
                  <th>Gestartet</th>
                  <th className="num">Dauer</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {shownRuns.map((r) => (
                  <tr key={r.id} className="click" onClick={() => nav(`/runs/${r.id}`)}>
                    <td>
                      <Link to={`/runs/${r.id}`} className="cell-title" onClick={(e) => e.stopPropagation()}>
                        {r.name || "Lauf"}
                      </Link>
                      <span className="mono faint" style={{ marginLeft: 8 }}>{shortId(r.id)}</span>
                    </td>
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
