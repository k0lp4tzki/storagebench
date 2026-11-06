// Thin REST client for the controller API. All calls are same-origin (the UI is
// served by the controller), so relative paths suffice.

import type {
  AgentView,
  EventRecord,
  RunDetail,
  RunSummary,
  StartRunRequest,
  MetricSample,
} from "./types";

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new Error(`${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text || `${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

export const api = {
  agents: () => getJSON<AgentView[]>("/api/agents"),
  runs: () => getJSON<RunSummary[]>("/api/runs"),
  run: (id: string) => getJSON<RunDetail>(`/api/runs/${encodeURIComponent(id)}`),
  runMetrics: (id: string) =>
    getJSON<Record<string, MetricSample[]>>(
      `/api/runs/${encodeURIComponent(id)}/metrics`,
    ),
  runEvents: (id: string) =>
    getJSON<EventRecord[]>(`/api/runs/${encodeURIComponent(id)}/events`),
  startRun: (req: StartRunRequest) =>
    postJSON<{ runId: string }>("/api/runs", req),
  stopRun: (id: string) =>
    postJSON<{ runId: string; action: string }>(
      `/api/runs/${encodeURIComponent(id)}/stop`,
      {},
    ),
  abortRun: (id: string) =>
    postJSON<{ runId: string; action: string }>(
      `/api/runs/${encodeURIComponent(id)}/abort`,
      {},
    ),
};
