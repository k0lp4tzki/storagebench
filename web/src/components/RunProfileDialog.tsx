import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import type { Profile } from "../api/types";
import { useLive } from "../state/LiveProvider";
import { profileSeconds } from "../domain";
import { fmtDuration } from "../format";
import { Modal } from "./Modal";
import { StatusBadge } from "./StatusBadge";

// RunProfileDialog: pick target hosts and start a profile run. Used from the
// dashboard, the profile list and the profile editor so it behaves identically
// everywhere.
export function RunProfileDialog({
  profile,
  onClose,
}: {
  profile: Profile;
  onClose: () => void;
}) {
  const nav = useNavigate();
  const { agents } = useLive();
  const usable = (a: (typeof agents)[number]) => a.status !== "offline" && a.capabilities.fio;
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(agents.filter((a) => a.status === "online" && a.capabilities.fio).map((a) => a.agentId)),
  );
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Agents often arrive after the first render (HTTP poll). Pre-select
  // online hosts once they show up, without undoing a manual choice.
  useEffect(() => {
    setSelected((prev) => {
      if (prev.size > 0) return prev;
      return new Set(
        agents.filter((a) => a.status === "online" && a.capabilities.fio).map((a) => a.agentId),
      );
    });
  }, [agents]);

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  async function start() {
    if (selected.size === 0) return;
    setBusy(true);
    setErr(null);
    try {
      const { batchId } = await api.startBatch({ profileId: profile.id, agentIds: [...selected] });
      onClose();
      nav(`/batches/${batchId}`);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  }

  const secs = profileSeconds(profile.steps);

  return (
    <Modal
      title={`„${profile.name}“ ausführen`}
      onClose={onClose}
      footer={
        <>
          <button className="btn secondary" onClick={onClose}>
            Abbrechen
          </button>
          <button className="btn" disabled={selected.size === 0 || busy} onClick={start}>
            {busy
              ? "Wird gestartet…"
              : selected.size === 0
                ? "Host auswählen"
                : `Auf ${selected.size} ${selected.size === 1 ? "Host" : "Hosts"} starten`}
          </button>
        </>
      }
    >
      <p className="muted" style={{ marginTop: 0 }}>
        {profile.steps.length} {profile.steps.length === 1 ? "Schritt läuft" : "Schritte laufen"} nacheinander,
        jeder Schritt gleichzeitig auf allen gewählten Hosts. Geschätzte Dauer{" "}
        <b className="mono" style={{ fontSize: 13 }}>{fmtDuration(secs)}</b>.
      </p>
      <div className="field-label" style={{ marginBottom: 8 }}>Ziel-Hosts</div>
      {agents.length === 0 && (
        <div className="callout">Keine Hosts verbunden. Starte einen Agenten, um Profile auszuführen.</div>
      )}
      {agents.map((a) => {
        const ok = usable(a);
        const on = selected.has(a.agentId);
        return (
          <label
            key={a.agentId}
            className={`host-pick ${on ? "on" : ""} ${ok ? "" : "disabled"}`}
          >
            <input type="checkbox" checked={on} disabled={!ok} onChange={() => toggle(a.agentId)} />
            <div className="grow">
              <div style={{ fontWeight: 580 }}>{a.system.hostname}</div>
              <div className="faint" style={{ fontSize: 12 }}>
                {a.capabilities.fio ? `fio ${a.capabilities.fioVersion ?? ""}` : "fio fehlt – nicht ausführbar"}
              </div>
            </div>
            <StatusBadge status={a.status} />
          </label>
        );
      })}
      {err && (
        <div className="callout err" style={{ marginTop: 12 }}>
          {err}
        </div>
      )}
    </Modal>
  );
}
