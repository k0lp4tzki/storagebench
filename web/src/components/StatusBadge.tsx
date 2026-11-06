import type { AgentStatus } from "../api/types";

const AGENT_LABEL: Record<AgentStatus, string> = {
  online: "online",
  degraded: "eingeschränkt",
  offline: "offline",
};
const AGENT_CLASS: Record<AgentStatus, string> = {
  online: "ok",
  degraded: "warn",
  offline: "err",
};

export function StatusBadge({ status }: { status: AgentStatus }) {
  return (
    <span className={`badge ${AGENT_CLASS[status] ?? ""}`}>
      <span className="dot" />
      {AGENT_LABEL[status] ?? status}
    </span>
  );
}

// RunStatusBadge maps run/batch lifecycle strings to a colored badge.
const RUN_CLASS: Record<string, string> = {
  completed: "ok",
  running: "accent",
  assigned: "accent",
  starting: "accent",
  created: "",
  pending: "",
  failed: "err",
  aborted: "warn",
  stopped: "warn",
};
const RUN_LABEL: Record<string, string> = {
  completed: "abgeschlossen",
  running: "läuft",
  assigned: "zugewiesen",
  starting: "startet",
  created: "erstellt",
  pending: "wartet",
  failed: "fehlgeschlagen",
  aborted: "abgebrochen",
  stopped: "gestoppt",
};

export function RunStatusBadge({ status }: { status: string }) {
  return (
    <span className={`badge ${RUN_CLASS[status] ?? ""}`}>
      <span className="dot" />
      {RUN_LABEL[status] ?? status}
    </span>
  );
}
