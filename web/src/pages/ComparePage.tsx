import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { CompareRun, FioSummaryValues } from "../api/types";
import { PageHeader, Empty } from "../components/PageHeader";
import { Icon } from "../components/Icon";
import { RunStatusBadge } from "../components/StatusBadge";
import { fmtBW, fmtDateTime, fmtIOPS, fmtLatUs, fmtPct, shortId } from "../format";

type Row = {
  key: keyof FioSummaryValues;
  label: string;
  fmt: (v?: number) => string;
  better: "max" | "min";
};

// Metrics shown in the run-level aggregate comparison, in display order.
const ROWS: Row[] = [
  { key: "iopsRead", label: "IOPS Read", fmt: fmtIOPS, better: "max" },
  { key: "iopsWrite", label: "IOPS Write", fmt: fmtIOPS, better: "max" },
  { key: "bwReadKBs", label: "Durchsatz Read", fmt: fmtBW, better: "max" },
  { key: "bwWriteKBs", label: "Durchsatz Write", fmt: fmtBW, better: "max" },
  { key: "latMeanUs", label: "Latenz Ø", fmt: fmtLatUs, better: "min" },
  { key: "latP95Us", label: "Latenz p95", fmt: fmtLatUs, better: "min" },
  { key: "latP99Us", label: "Latenz p99", fmt: fmtLatUs, better: "min" },
  { key: "latP999Us", label: "Latenz p99.9", fmt: fmtLatUs, better: "min" },
];

export function ComparePage() {
  const [params] = useSearchParams();
  const ids = useMemo(
    () =>
      (params.get("runs") || "")
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean),
    [params],
  );
  const [runs, setRuns] = useState<CompareRun[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (ids.length === 0) {
      setLoading(false);
      return;
    }
    setLoading(true);
    api
      .compare(ids)
      .then((r) => setRuns(r ?? []))
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, [ids]);

  // For each aggregate row, find the best value across runs (for highlighting).
  const best = useMemo(() => {
    const b: Partial<Record<keyof FioSummaryValues, number>> = {};
    if (runs.length < 2) return b;
    for (const row of ROWS) {
      const vals = runs
        .map((r) => r.aggregate[row.key])
        .filter((v): v is number => typeof v === "number" && v > 0);
      if (vals.length < 2) continue;
      b[row.key] = row.better === "max" ? Math.max(...vals) : Math.min(...vals);
    }
    return b;
  }, [runs]);

  return (
    <div className="stack">
      <PageHeader
        crumbs={[{ to: "/results?tab=runs", label: "Ergebnisse" }]}
        title="Vergleich"
        meta={<span>{ids.length} ausgewählte Läufe gegenübergestellt</span>}
        actions={
          runs.length > 0 ? (
            <a className="btn secondary" href={api.compareExportCsvUrl(ids)} download>
              <Icon name="down" size={13} /> CSV exportieren
            </a>
          ) : undefined
        }
      />

      {err && (
        <div className="callout err">
          <Icon name="alert" />
          <div className="body">{err}</div>
        </div>
      )}

      {loading ? (
        <div className="panel empty">Lädt…</div>
      ) : ids.length === 0 ? (
        <Empty title="Keine Läufe ausgewählt">
          Wähle in den{" "}
          <Link to="/results?tab=runs" style={{ color: "var(--accent-text)" }}>
            Ergebnissen
          </Link>{" "}
          mehrere Läufe aus und klicke „Vergleichen".
        </Empty>
      ) : runs.length === 0 ? (
        <Empty title="Keine Daten" compact>
          Die ausgewählten Läufe wurden nicht gefunden.
        </Empty>
      ) : (
        <>
          {/* Run headers */}
          <section className="panel">
            <div className="panel-head">
              <h2>Ausgewählte Läufe</h2>
            </div>
            <div className="table-wrap">
              <table className="data">
                <thead>
                  <tr>
                    <th>Lauf</th>
                    <th>Status</th>
                    <th>Gestartet</th>
                    <th className="num">Hosts</th>
                  </tr>
                </thead>
                <tbody>
                  {runs.map((r) => (
                    <tr key={r.id}>
                      <td>
                        <Link to={`/runs/${r.id}`} className="cell-title">
                          {r.name || "Lauf"}
                        </Link>
                        <span className="mono faint" style={{ marginLeft: 8 }}>
                          {shortId(r.id)}
                        </span>
                      </td>
                      <td>
                        <RunStatusBadge status={r.status} />
                      </td>
                      <td>{fmtDateTime(r.startedAt)}</td>
                      <td className="num">{r.nodes.length}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          {/* Aggregate comparison (metric per row, run per column) */}
          <section className="panel">
            <div className="panel-head">
              <h2>Gesamtwerte je Lauf</h2>
              {runs.length > 1 && (
                <span className="sub">
                  <span style={{ color: "var(--ok)", fontWeight: 600 }}>Grün</span> = bester Wert der
                  Zeile
                </span>
              )}
            </div>
            <div className="table-wrap">
              <table className="data">
                <thead>
                  <tr>
                    <th>Kennzahl</th>
                    {runs.map((r) => (
                      <th key={r.id} className="num">
                        {r.name || shortId(r.id)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {ROWS.map((row) => (
                    <tr key={row.key}>
                      <td className="cell-title">{row.label}</td>
                      {runs.map((r) => {
                        const v = r.aggregate[row.key];
                        const isBest = v != null && best[row.key] === v && v > 0;
                        return (
                          <td key={r.id} className={`num ${isBest ? "best" : ""}`}>
                            {row.fmt(v)}
                          </td>
                        );
                      })}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>

          {/* Per-host breakdown per run */}
          {runs.map((r) => (
            <section className="panel" key={r.id}>
              <div className="panel-head">
                <h2>{r.name || "Lauf"} · je Host</h2>
                <span className="sub mono">{shortId(r.id)}</span>
              </div>
              {r.nodes.length === 0 ? (
                <Empty title="Keine Hosts" compact />
              ) : (
                <div className="table-wrap">
                  <table className="data">
                    <thead>
                      <tr>
                        <th>Host</th>
                        <th className="num">IOPS R</th>
                        <th className="num">IOPS W</th>
                        <th className="num">Durchs. R</th>
                        <th className="num">Latenz p99</th>
                        <th className="num">p99.9</th>
                        <th className="num" title="Schwankung der IOPS über die Zeit">
                          Schwankung
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {r.nodes.map((n) => (
                        <tr key={n.agentId}>
                          <td className="cell-title">{n.hostname}</td>
                          <td className="num">{fmtIOPS(n.summary.iopsRead)}</td>
                          <td className="num">{fmtIOPS(n.summary.iopsWrite)}</td>
                          <td className="num">{fmtBW(n.summary.bwReadKBs)}</td>
                          <td className="num">{fmtLatUs(n.summary.latP99Us)}</td>
                          <td className="num">{fmtLatUs(n.summary.latP999Us)}</td>
                          <td className="num">
                            {n.iopsCv < 0 ? (
                              <span className="faint">–</span>
                            ) : (
                              <span
                                className={`badge ${
                                  n.iopsCv < 0.1 ? "ok" : n.iopsCv < 0.25 ? "warn" : "err"
                                }`}
                              >
                                ±{fmtPct(n.iopsCv)}
                              </span>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          ))}
        </>
      )}
    </div>
  );
}
