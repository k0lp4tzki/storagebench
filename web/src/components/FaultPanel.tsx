import { useEffect, useMemo, useState } from "react";
import { api } from "../api/client";
import type { AgentView, FaultKind, FaultRecord, FaultSpec, RunNodeDetail } from "../api/types";
import { Icon } from "./Icon";
import { Modal } from "./Modal";

const KIND_LABEL: Record<FaultKind, string> = {
  latency: "Latenz",
  loss: "Paketverlust",
  bandwidth: "Bandbreitenlimit",
  partition: "Partition (Verbindung kappen)",
};

// fmtCountdown renders remaining milliseconds as mm:ss (or ss s under a minute).
function fmtCountdown(ms: number): string {
  const total = Math.ceil(ms / 1000);
  if (total < 60) return `${total}s`;
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

interface Props {
  runId: string;
  nodes: RunNodeDetail[];
  agents: AgentView[];
  faults: FaultRecord[];
  onChanged: () => void;
}

// FaultPanel lets the operator inject and revert network faults against a host
// participating in the run. Only hosts whose agent advertises the fault-injection
// capability (a reachable privilege-broker) can be targeted.
export function FaultPanel({ runId, nodes, agents, faults, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const active = faults.filter((f) => f.tsRevertMilli === 0);
  const done = faults.filter((f) => f.tsRevertMilli > 0);

  // Tick every second while a fault is active so the countdown stays live. When
  // a TTL elapses we refresh so the controller's expiry stamp is picked up and
  // the fault moves to "finished".
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (active.length === 0) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [active.length]);
  useEffect(() => {
    // When any active fault's TTL has elapsed, pull fresh data (the controller
    // stamps ts_revert at expiry) so the UI reflects the finished state.
    const expired = active.some(
      (f) => f.ttlSeconds > 0 && now >= f.tsInjectMilli + f.ttlSeconds * 1000 + 1500,
    );
    if (expired) onChanged();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [now]);

  // Hosts eligible for faults: in this run AND fault-injection capable + online.
  const capable = useMemo(() => {
    const byId = new Map(agents.map((a) => [a.agentId, a]));
    return nodes.filter((n) => {
      const a = byId.get(n.agentId);
      return a?.capabilities.faultInjection && a.connected;
    });
  }, [nodes, agents]);

  async function revert(f: FaultRecord) {
    setBusy(true);
    setErr(null);
    try {
      await api.revertFault(runId, { agentId: f.agentId, iface: f.iface });
      onChanged();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    } finally {
      setBusy(false);
    }
  }

  const hostName = (agentId: string) =>
    nodes.find((n) => n.agentId === agentId)?.hostname ?? agentId;

  return (
    <section className="panel">
      <div className="panel-head">
        <h2>
          <Icon name="alert" size={15} /> Störungen (Resilienz)
        </h2>
        {capable.length > 0 && (
          <button className="btn secondary sm" onClick={() => setOpen(true)}>
            <Icon name="plus" size={13} /> Störung einspielen
          </button>
        )}
      </div>

      <div className="panel-body stack" style={{ gap: 12 }}>
        {err && (
          <div className="callout err">
            <Icon name="alert" />
            <div className="body">{err}</div>
          </div>
        )}

        {capable.length === 0 && (
          <span className="faint">
            Kein Host in diesem Lauf unterstützt Fault-Injection (es ist kein
            Privilege-Broker erreichbar).
          </span>
        )}

        {capable.length > 0 && active.length === 0 && (
          <span className="faint">Aktuell keine aktive Störung.</span>
        )}

        {active.length > 0 && (
          <div className="table-wrap">
            <table className="data">
              <thead>
                <tr>
                  <th>Host</th>
                  <th>Störung</th>
                  <th>Interface</th>
                  <th>Verbleibend</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {active.map((f) => {
                  const remainMs =
                    f.ttlSeconds > 0 ? f.tsInjectMilli + f.ttlSeconds * 1000 - now : -1;
                  return (
                    <tr key={f.id}>
                      <td className="cell-title">{hostName(f.agentId)}</td>
                      <td>
                        <span className="badge warn">{f.detail || f.kind}</span>
                      </td>
                      <td className="mono">{f.iface}</td>
                      <td className="mono">
                        {f.ttlSeconds <= 0 ? (
                          <span className="faint">kein TTL</span>
                        ) : remainMs > 0 ? (
                          fmtCountdown(remainMs)
                        ) : (
                          <span className="faint">läuft aus…</span>
                        )}
                      </td>
                      <td className="num">
                        <button className="btn ghost sm" disabled={busy} onClick={() => revert(f)}>
                          Zurücknehmen
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}

        {done.length > 0 && (
          <details>
            <summary className="faint" style={{ cursor: "pointer", fontSize: 13 }}>
              Abgeschlossene Störungen ({done.length})
            </summary>
            <div className="table-wrap" style={{ marginTop: 8 }}>
              <table className="data">
                <thead>
                  <tr>
                    <th>Host</th>
                    <th>Störung</th>
                    <th>Interface</th>
                    <th>Dauer</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {done.map((f) => (
                    <tr key={f.id}>
                      <td className="cell-title">{hostName(f.agentId)}</td>
                      <td>{f.detail || f.kind}</td>
                      <td className="mono">{f.iface}</td>
                      <td className="mono">
                        {fmtCountdown(Math.max(0, f.tsRevertMilli - f.tsInjectMilli))}
                      </td>
                      <td>
                        <span className="badge ok">abgeschlossen</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </details>
        )}
      </div>

      {open && (
        <InjectDialog
          runId={runId}
          hosts={capable}
          onClose={() => setOpen(false)}
          onDone={() => {
            setOpen(false);
            onChanged();
          }}
        />
      )}
    </section>
  );
}

function InjectDialog({
  runId,
  hosts,
  onClose,
  onDone,
}: {
  runId: string;
  hosts: RunNodeDetail[];
  onClose: () => void;
  onDone: () => void;
}) {
  const [agentId, setAgentId] = useState(hosts[0]?.agentId ?? "");
  const [kind, setKind] = useState<FaultKind>("latency");
  const [iface, setIface] = useState("");
  const [delayMs, setDelayMs] = useState(100);
  const [jitterMs, setJitterMs] = useState(10);
  const [lossPercent, setLossPercent] = useState(5);
  const [rateKbit, setRateKbit] = useState(10000);
  const [ttl, setTtl] = useState(300);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  function buildSpec(): FaultSpec {
    const base: FaultSpec = { kind, iface: iface.trim() };
    if (kind === "latency") {
      base.delayMs = delayMs;
      if (jitterMs > 0) base.jitterMs = jitterMs;
    } else if (kind === "loss") {
      base.lossPercent = lossPercent;
    } else if (kind === "bandwidth") {
      base.rateKbit = rateKbit;
    }
    return base;
  }

  async function submit() {
    if (!agentId) {
      setErr("Bitte einen Host wählen.");
      return;
    }
    if (!iface.trim()) {
      setErr("Bitte das Interface angeben (z. B. eth1). Das Management-Interface ist geschützt.");
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      await api.injectFault(runId, { agentId, spec: buildSpec(), ttlSeconds: ttl });
      onDone();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
      setBusy(false);
    }
  }

  const danger = kind === "partition";

  return (
    <Modal
      title="Netzwerk-Störung einspielen"
      onClose={onClose}
      width={520}
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>
            Abbrechen
          </button>
          <button
            className={`btn ${danger ? "danger" : ""}`}
            disabled={busy}
            onClick={submit}
            data-autofocus
          >
            {busy ? "Spiele ein…" : "Einspielen"}
          </button>
        </>
      }
    >
      <div className="stack" style={{ gap: 14 }}>
        <div className="field">
          <label className="field-label">Host</label>
          <select value={agentId} onChange={(e) => setAgentId(e.target.value)}>
            {hosts.map((h) => (
              <option key={h.agentId} value={h.agentId}>
                {h.hostname}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <label className="field-label">Art der Störung</label>
          <select value={kind} onChange={(e) => setKind(e.target.value as FaultKind)}>
            {(Object.keys(KIND_LABEL) as FaultKind[]).map((k) => (
              <option key={k} value={k}>
                {KIND_LABEL[k]}
              </option>
            ))}
          </select>
        </div>

        <div className="field">
          <label className="field-label">Interface</label>
          <input
            value={iface}
            placeholder="z. B. eth1 (nicht das Management-Interface)"
            onChange={(e) => setIface(e.target.value)}
          />
          <span className="hint">
            Das Interface der Controller-Verbindung ist automatisch geschützt und
            wird abgelehnt.
          </span>
        </div>

        {kind === "latency" && (
          <div className="row" style={{ gap: 14 }}>
            <div className="field">
              <label className="field-label">Verzögerung (ms)</label>
              <input type="number" min={1} value={delayMs} onChange={(e) => setDelayMs(+e.target.value)} />
            </div>
            <div className="field">
              <label className="field-label">Jitter (ms)</label>
              <input type="number" min={0} value={jitterMs} onChange={(e) => setJitterMs(+e.target.value)} />
            </div>
          </div>
        )}

        {kind === "loss" && (
          <div className="field">
            <label className="field-label">Paketverlust (%)</label>
            <input
              type="number"
              min={0.1}
              max={100}
              step={0.1}
              value={lossPercent}
              onChange={(e) => setLossPercent(+e.target.value)}
            />
          </div>
        )}

        {kind === "bandwidth" && (
          <div className="field">
            <label className="field-label">Bandbreitenlimit (kbit/s)</label>
            <input type="number" min={1} value={rateKbit} onChange={(e) => setRateKbit(+e.target.value)} />
          </div>
        )}

        {danger && (
          <div className="callout err">
            <Icon name="alert" />
            <div className="body">
              Eine Partition kappt den gesamten Verkehr auf diesem Interface. Das
              Management-Interface bleibt geschützt, sodass die Controller-Verbindung
              erhalten bleibt.
            </div>
          </div>
        )}

        <div className="field">
          <label className="field-label">Automatisch zurücknehmen nach (Sekunden)</label>
          <input type="number" min={1} value={ttl} onChange={(e) => setTtl(+e.target.value)} />
          <span className="hint">
            Sicherheitsnetz: Der Broker nimmt die Störung spätestens nach dieser
            Zeit automatisch zurück (Watchdog).
          </span>
        </div>

        {err && (
          <div className="callout err">
            <Icon name="alert" />
            <div className="body">{err}</div>
          </div>
        )}
      </div>
    </Modal>
  );
}
