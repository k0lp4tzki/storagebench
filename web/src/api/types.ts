// TypeScript mirrors of the Go API/JSON shapes. Kept in one place so the UI and
// backend contracts stay aligned. Field names match the Go json tags.

export type AgentStatus = "online" | "degraded" | "offline";

export interface SystemInfo {
  hostname: string;
  os: string;
  distro?: string;
  kernel?: string;
  arch: string;
  cpuModel?: string;
  cpuCores?: number;
  memTotalMB?: number;
  agentVersion: string;
}

export interface Capabilities {
  fio: boolean;
  fioVersion?: string;
  ioUring: boolean;
  libaio: boolean;
  multipath: boolean;
  deviceMapper: boolean;
  tc: boolean;
  smartctl: boolean;
  nvme: boolean;
  iostat: boolean;
  ioEngines?: string[];
}

export interface AgentView {
  agentId: string;
  status: AgentStatus;
  connected: boolean;
  system: SystemInfo;
  capabilities: Capabilities;
  lastSeenMs: number;
  rttMs: number;
  clockOffsetMs: number;
}

export interface MetricSample {
  t: number; // ts wall millis
  m?: number; // mono nanos
  ior?: number; // iops read
  iow?: number; // iops write
  bwr?: number; // bw read KiB/s
  bww?: number; // bw write KiB/s
  lm?: number; // lat mean us
  l95?: number; // lat p95 us
  l99?: number; // lat p99 us
  qd?: number;
  cpu?: number;
  mem?: number;
  load?: number;
  du?: number;
  nrx?: number;
  ntx?: number;
}

export interface RunSummary {
  id: string;
  name: string;
  status: string;
  startedAt: string;
  endedAt: string;
}

export interface FioSummary {
  iopsRead: number;
  iopsWrite: number;
  bwReadKBs: number;
  bwWriteKBs: number;
  latMeanUs: number;
  latP95Us: number;
  latP99Us: number;
}

export interface RunNodeDetail {
  agentId: string;
  hostname: string;
  phase: string;
  phaseMessage: string;
  systemSnapshot: unknown;
  fioSummary?: FioSummary | null;
}

export interface RunDetail {
  id: string;
  name: string;
  status: string;
  startedAt: string;
  endedAt: string;
  // The frozen per-agent spec map (agentId -> Spec). Used to restart a run.
  resolvedConfig: Record<string, Spec> | null;
  nodes: RunNodeDetail[];
}

export interface EventRecord {
  agentId: string;
  tsWallMilli: number;
  severity: string;
  source: string;
  message: string;
}

// Benchmark spec, mirrors internal/benchmark.Spec.
export type Workload =
  | "randread"
  | "randwrite"
  | "randrw"
  | "read"
  | "write"
  | "rw";

export type IOEngine = "libaio" | "io_uring" | "psync" | "sync";
export type TargetKind = "file" | "device";

export interface Spec {
  name: string;
  targetKind: TargetKind;
  targetPath: string;
  fileSizeMiB?: number;
  sizeMiB?: number;
  workload: Workload;
  blockSize: string;
  rwMixRead?: number;
  ioDepth: number;
  numJobs: number;
  engine: IOEngine;
  directIO: boolean;
  runtimeSeconds: number;
  rampSeconds?: number;
  targetIOPS?: number;
  maxBandwidthKBs?: number;
  allowDestructive?: boolean;
}

export interface StartRunRequest {
  name: string;
  specs: Record<string, Spec>; // agentId -> spec
  authorizationSeconds?: number;
}

// --- Profiles & batches ---

export interface ProfileStep {
  spec: Spec;
}

export interface Profile {
  id: string;
  name: string;
  description: string;
  steps: ProfileStep[];
  createdAt: string;
  updatedAt: string;
}

export interface Batch {
  id: string;
  profileId: string;
  name: string;
  status: string;
  agentIds: string[];
  totalSteps: number;
  stepIndex: number;
  startedAt: string;
  endedAt: string;
}

export interface BatchRunEntry {
  runId: string;
  step: number;
  name: string;
  status: string;
  startedAt: string;
  endedAt: string;
}

export interface BatchDetail extends Batch {
  profileName: string;
  runs: BatchRunEntry[];
}

export interface StartBatchRequest {
  profileId: string;
  agentIds: string[];
  name?: string;
}

// UI WebSocket message envelope.
export interface UIMessage {
  type: "metrics" | "event" | "testState" | "agents" | "batch";
  payload: unknown;
}

export interface UIBatchPayload {
  batchId: string;
  status: string;
  stepIndex: number;
  totalSteps: number;
  runId?: string;
}

export interface UIMetricsPayload {
  agentId: string;
  runId: string;
  samples: MetricSample[];
}

export interface UITestStatePayload {
  agentId: string;
  state: { runId: string; phase: string; message?: string };
}

export interface UIEventPayload {
  agentId: string;
  event: {
    runId?: string;
    t: number;
    sev: string;
    src: string;
    msg: string;
  };
}
