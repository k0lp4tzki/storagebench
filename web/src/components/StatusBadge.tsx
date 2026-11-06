import type { AgentStatus } from "../api/types";
import { statusInfo } from "../domain";

const AGENT: Record<AgentStatus, { label: string; tone: string }> = {
  online: { label: "Online", tone: "ok" },
  degraded: { label: "Eingeschränkt", tone: "warn" },
  offline: { label: "Offline", tone: "err" },
};

export function StatusBadge({ status }: { status: AgentStatus }) {
  const s = AGENT[status] ?? { label: status, tone: "" };
  return (
    <span className={`badge ${s.tone}`}>
      <span className="dot" />
      {s.label}
    </span>
  );
}

// RunStatusBadge maps run/batch/phase lifecycle strings to a labelled badge.
// The label always carries the meaning; colour only reinforces it.
export function RunStatusBadge({ status }: { status: string }) {
  const s = statusInfo(status);
  return (
    <span className={`badge ${s.tone}`}>
      <span className="dot" />
      {s.label}
    </span>
  );
}
