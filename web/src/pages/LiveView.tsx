import { useLive } from "../state/LiveProvider";
import { TimeChart } from "../components/TimeChart";
import { fmtBW, fmtIOPS, fmtLatUs } from "../format";

export function LiveView() {
  const { agents, liveMetrics, testStates } = useLive();

  return (
    <div>
      <h1>Live-Ansicht</h1>
      <section className="grid3">
        {agents.length === 0 && (
          <div className="card muted">Keine Agenten verbunden.</div>
        )}
        {agents.map((a) => {
          const samples = liveMetrics[a.agentId] ?? [];
          const last = samples[samples.length - 1];
          const ts = testStates[a.agentId];
          const iopsRead = samples.map((s) => s.ior ?? 0);
          const iopsWrite = samples.map((s) => s.iow ?? 0);
          const latMean = samples.map((s) => s.lm ?? 0);
          const latP99 = samples.map((s) => s.l99 ?? 0);

          return (
            <div className="card" key={a.agentId}>
              <div className="host">{a.system.hostname}</div>
              <div className="sub">
                {ts ? `${ts.phase} · ${ts.runId.slice(0, 8)}` : "idle"}
              </div>

              <div className="metrics-row">
                <div className="metric">
                  <div className="label">IOPS R</div>
                  <div className="value">{fmtIOPS(last?.ior)}</div>
                </div>
                <div className="metric">
                  <div className="label">IOPS W</div>
                  <div className="value">{fmtIOPS(last?.iow)}</div>
                </div>
                <div className="metric">
                  <div className="label">p99</div>
                  <div className="value">{fmtLatUs(last?.l99)}</div>
                </div>
              </div>

              <div style={{ marginTop: 14 }}>
                <TimeChart
                  series={[
                    { values: iopsRead, color: "var(--read)", label: "IOPS Read" },
                    { values: iopsWrite, color: "var(--write)", label: "IOPS Write" },
                  ]}
                />
              </div>
              <div style={{ marginTop: 14 }}>
                <TimeChart
                  unit="µs"
                  series={[
                    { values: latMean, color: "var(--accent)", label: "Latenz Ø" },
                    { values: latP99, color: "var(--err)", label: "Latenz p99" },
                  ]}
                />
              </div>

              <div className="sub" style={{ marginTop: 10 }}>
                Bandbreite: {fmtBW(last?.bwr)} lesen / {fmtBW(last?.bww)} schreiben
              </div>
            </div>
          );
        })}
      </section>
    </div>
  );
}
