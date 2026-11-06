import { useLive } from "../state/LiveProvider";
import { Chart } from "../components/Chart";
import { RunStatusBadge } from "../components/StatusBadge";
import { fmtBW, fmtIOPS, fmtLatUs, secondsFromStart } from "../format";

export function LiveView() {
  const { agents, liveMetrics, testStates } = useLive();

  const anyActive = agents.some((a) => {
    const s = testStates[a.agentId];
    return s && ["running", "starting", "assigned"].includes(s.phase);
  });

  return (
    <div>
      {!anyActive && (
        <div className="card" style={{ marginBottom: 18 }}>
          <div className="muted">
            Kein aktiver Test. Starte ein Profil unter „Profile", um Live-Daten zu
            sehen.
          </div>
        </div>
      )}

      <div className="grid cols-3">
        {agents.length === 0 && (
          <div className="card empty">Keine Agenten verbunden.</div>
        )}
        {agents.map((a) => {
          const samples = liveMetrics[a.agentId] ?? [];
          const last = samples[samples.length - 1];
          const ts = testStates[a.agentId];
          const x = secondsFromStart(samples.map((s) => s.t));

          return (
            <div className="card" key={a.agentId}>
              <div className="toolbar" style={{ marginBottom: 6 }}>
                <div style={{ fontWeight: 650 }}>{a.system.hostname}</div>
                <div className="spacer" />
                {ts ? <RunStatusBadge status={ts.phase} /> : <span className="badge">idle</span>}
              </div>

              <div className="row" style={{ marginBottom: 10 }}>
                <div className="stat">
                  <div className="label">IOPS R/W</div>
                  <div style={{ fontWeight: 650, fontVariantNumeric: "tabular-nums" }}>
                    {fmtIOPS(last?.ior)} / {fmtIOPS(last?.iow)}
                  </div>
                </div>
                <div className="stat">
                  <div className="label">Latenz p99</div>
                  <div style={{ fontWeight: 650, fontVariantNumeric: "tabular-nums" }}>
                    {fmtLatUs(last?.l99)}
                  </div>
                </div>
              </div>

              <div className="faint" style={{ fontSize: 11, margin: "4px 0 2px" }}>
                IOPS
              </div>
              <Chart
                x={x}
                height={150}
                yFormat={(v) => fmtIOPS(v)}
                series={[
                  { label: "Read", data: samples.map((s) => s.ior ?? 0), color: "var(--read)" },
                  { label: "Write", data: samples.map((s) => s.iow ?? 0), color: "var(--write)" },
                ]}
              />

              <div className="faint" style={{ fontSize: 11, margin: "10px 0 2px" }}>
                Latenz (µs)
              </div>
              <Chart
                x={x}
                height={150}
                yFormat={(v) => fmtLatUs(v)}
                series={[
                  { label: "Ø", data: samples.map((s) => s.lm ?? 0), color: "var(--accent)" },
                  { label: "p99", data: samples.map((s) => s.l99 ?? 0), color: "var(--err)" },
                ]}
              />

              <div className="faint" style={{ fontSize: 12, marginTop: 10 }}>
                Durchsatz: {fmtBW(last?.bwr)} lesen · {fmtBW(last?.bww)} schreiben
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
