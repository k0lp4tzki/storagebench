import type { AgentStatus } from "../api/types";

const LABEL: Record<AgentStatus, string> = {
  online: "online",
  degraded: "eingeschränkt",
  offline: "offline",
};

export function StatusBadge({ status }: { status: AgentStatus }) {
  return (
    <span className="badge">
      <span className={`dot ${status}`} />
      {LABEL[status] ?? status}
    </span>
  );
}
