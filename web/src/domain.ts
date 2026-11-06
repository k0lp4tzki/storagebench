// Domain helpers shared by pages: human labels for fio parameters, status
// semantics and profile arithmetic. Keeping them here makes every page speak
// the same language.

import type { Profile, ProfileStep, Spec, Workload } from "./api/types";

export const WORKLOAD_LABEL: Record<Workload, string> = {
  randread: "Random Read",
  randwrite: "Random Write",
  randrw: "Random Mixed",
  read: "Seq. Read",
  write: "Seq. Write",
  rw: "Seq. Mixed",
};

export function workloadLabel(s: Spec): string {
  const base = WORKLOAD_LABEL[s.workload] ?? s.workload;
  if (s.workload === "randrw" || s.workload === "rw") {
    const r = s.rwMixRead ?? 70;
    return `${base} ${r}/${100 - r}`;
  }
  return base;
}

// specSummary: compact one-liner, e.g. "4k · QD32 × 4 · 60 s".
export function specSummary(s: Spec): string {
  return `${s.blockSize} · QD${s.ioDepth}×${s.numJobs} · ${s.runtimeSeconds} s`;
}

export function stepSeconds(s: Spec): number {
  return (s.runtimeSeconds || 0) + (s.rampSeconds || 0);
}

export function profileSeconds(steps: ProfileStep[]): number {
  return steps.reduce((a, st) => a + stepSeconds(st.spec), 0);
}

export function profileOf(p: Profile) {
  return { seconds: profileSeconds(p.steps), count: p.steps.length };
}

export const isWriteWorkload = (w: Workload) =>
  w === "randwrite" || w === "write" || w === "randrw" || w === "rw";

// ---- Status semantics ------------------------------------------------------

export type Tone = "ok" | "warn" | "err" | "accent" | "";

const STATUS: Record<string, { label: string; tone: Tone }> = {
  completed: { label: "Abgeschlossen", tone: "ok" },
  running: { label: "Läuft", tone: "accent" },
  assigned: { label: "Zugewiesen", tone: "accent" },
  starting: { label: "Startet", tone: "accent" },
  stopping: { label: "Stoppt", tone: "accent" },
  aborting: { label: "Bricht ab", tone: "accent" },
  created: { label: "Erstellt", tone: "" },
  pending: { label: "Wartet", tone: "" },
  failed: { label: "Fehlgeschlagen", tone: "err" },
  aborted: { label: "Abgebrochen", tone: "warn" },
  stopped: { label: "Gestoppt", tone: "warn" },
  idle: { label: "Bereit", tone: "" },
};

export function statusInfo(s: string | undefined): { label: string; tone: Tone } {
  if (!s) return { label: "–", tone: "" };
  return STATUS[s] ?? { label: s, tone: "" };
}

export const ACTIVE_PHASES = new Set(["running", "starting", "assigned", "stopping", "aborting"]);
export const ACTIVE_RUN = new Set(["running", "assigned", "starting", "pending", "created"]);

// Result filter buckets used on the results page.
export type Bucket = "all" | "ok" | "failed" | "aborted" | "active";
export function bucketOf(status: string): Bucket {
  if (status === "completed") return "ok";
  if (status === "failed") return "failed";
  if (status === "aborted" || status === "stopped") return "aborted";
  if (ACTIVE_RUN.has(status)) return "active";
  return "all";
}
