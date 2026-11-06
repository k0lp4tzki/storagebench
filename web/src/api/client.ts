// Thin REST client for the controller API. All calls are same-origin (the UI is
// served by the controller), so relative paths suffice.

import type {
  AgentView,
  Batch,
  BatchDetail,
  CompareRun,
  EventRecord,
  FaultRecord,
  InjectFaultRequest,
  Profile,
  RevertFaultRequest,
  RunDetail,
  RunSummary,
  StartBatchRequest,
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

async function sendJSON<T>(
  method: "POST" | "PUT" | "DELETE",
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => "");
    let msg = text;
    try {
      msg = JSON.parse(text).error ?? text;
    } catch {
      /* keep raw text */
    }
    throw new Error(msg || `${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

const postJSON = <T,>(path: string, body: unknown) =>
  sendJSON<T>("POST", path, body);

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

  // Profiles
  profiles: () => getJSON<Profile[]>("/api/profiles"),
  profile: (id: string) =>
    getJSON<Profile>(`/api/profiles/${encodeURIComponent(id)}`),
  createProfile: (p: Omit<Profile, "id" | "createdAt" | "updatedAt">) =>
    postJSON<Profile>("/api/profiles", p),
  updateProfile: (id: string, p: Omit<Profile, "createdAt" | "updatedAt">) =>
    sendJSON<Profile>("PUT", `/api/profiles/${encodeURIComponent(id)}`, p),
  deleteProfile: (id: string) =>
    sendJSON<{ deleted: string }>(
      "DELETE",
      `/api/profiles/${encodeURIComponent(id)}`,
    ),

  // Batches
  batches: () => getJSON<Batch[]>("/api/batches"),
  batch: (id: string) =>
    getJSON<BatchDetail>(`/api/batches/${encodeURIComponent(id)}`),
  startBatch: (req: StartBatchRequest) =>
    postJSON<{ batchId: string }>("/api/batches", req),

  // Comparison + CSV export (Phase 5)
  compare: (ids: string[]) =>
    getJSON<CompareRun[]>(`/api/compare?runs=${encodeURIComponent(ids.join(","))}`),
  runExportCsvUrl: (id: string) => `/api/runs/${encodeURIComponent(id)}/export.csv`,
  compareExportCsvUrl: (ids: string[]) =>
    `/api/compare/export.csv?runs=${encodeURIComponent(ids.join(","))}`,

  // Faults (resilience testing)
  runFaults: (id: string) =>
    getJSON<FaultRecord[]>(`/api/runs/${encodeURIComponent(id)}/faults`),
  injectFault: (id: string, req: InjectFaultRequest) =>
    postJSON<{ faultId: string; status: string }>(
      `/api/runs/${encodeURIComponent(id)}/faults`,
      req,
    ),
  revertFault: (id: string, req: RevertFaultRequest) =>
    postJSON<{ status: string }>(
      `/api/runs/${encodeURIComponent(id)}/faults/revert`,
      req,
    ),
};
