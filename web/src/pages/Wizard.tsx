import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useLive } from "../state/LiveProvider";
import { api } from "../api/client";
import type { IOEngine, Spec, StartRunRequest, TargetKind, Workload } from "../api/types";

const WORKLOADS: { value: Workload; label: string }[] = [
  { value: "randread", label: "Zufällig Lesen (randread)" },
  { value: "randwrite", label: "Zufällig Schreiben (randwrite)" },
  { value: "randrw", label: "Zufällig gemischt (randrw)" },
  { value: "read", label: "Sequenziell Lesen (read)" },
  { value: "write", label: "Sequenziell Schreiben (write)" },
  { value: "rw", label: "Sequenziell gemischt (rw)" },
];

const BLOCK_SIZES = [
  "512",
  "1k",
  "4k",
  "8k",
  "16k",
  "32k",
  "64k",
  "128k",
  "256k",
  "512k",
  "1m",
  "2m",
  "4m",
];

const ENGINES: IOEngine[] = ["libaio", "io_uring", "psync", "sync"];

interface WizardSpec {
  name: string;
  targetKind: TargetKind;
  targetPath: string;
  fileSizeMiB: number;
  workload: Workload;
  blockSize: string;
  rwMixRead: number;
  ioDepth: number;
  numJobs: number;
  engine: IOEngine;
  directIO: boolean;
  runtimeSeconds: number;
  rampSeconds: number;
  targetIOPS: number;
  allowDestructive: boolean;
}

const DEFAULTS: WizardSpec = {
  name: "Benchmark",
  targetKind: "file",
  targetPath: "/var/tmp/storagebench.test",
  fileSizeMiB: 1024,
  workload: "randread",
  blockSize: "4k",
  rwMixRead: 70,
  ioDepth: 32,
  numJobs: 4,
  engine: "libaio",
  directIO: true,
  runtimeSeconds: 60,
  rampSeconds: 5,
  targetIOPS: 0,
  allowDestructive: false,
};

function toSpec(w: WizardSpec): Spec {
  const spec: Spec = {
    name: w.name,
    targetKind: w.targetKind,
    targetPath: w.targetPath,
    workload: w.workload,
    blockSize: w.blockSize,
    ioDepth: w.ioDepth,
    numJobs: w.numJobs,
    engine: w.engine,
    directIO: w.directIO,
    runtimeSeconds: w.runtimeSeconds,
    rampSeconds: w.rampSeconds,
  };
  if (w.targetKind === "file") spec.fileSizeMiB = w.fileSizeMiB;
  if (w.workload === "randrw" || w.workload === "rw") spec.rwMixRead = w.rwMixRead;
  if (w.targetIOPS > 0) spec.targetIOPS = w.targetIOPS;
  if (w.allowDestructive) spec.allowDestructive = true;
  return spec;
}

const isMixed = (w: Workload) => w === "randrw" || w === "rw";
const isWrite = (w: Workload) =>
  w === "randwrite" || w === "write" || w === "randrw" || w === "rw";

export function Wizard() {
  const { agents } = useLive();
  const nav = useNavigate();
  const [step, setStep] = useState(0);
  const [spec, setSpec] = useState<WizardSpec>(DEFAULTS);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const set = <K extends keyof WizardSpec>(k: K, v: WizardSpec[K]) =>
    setSpec((s) => ({ ...s, [k]: v }));

  const deviceWriteWithoutOptIn =
    spec.targetKind === "device" && isWrite(spec.workload) && !spec.allowDestructive;

  const canSubmit = useMemo(
    () => selected.size > 0 && !deviceWriteWithoutOptIn && spec.name.trim() !== "",
    [selected, deviceWriteWithoutOptIn, spec.name],
  );

  const toggleAgent = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });

  async function submit() {
    setErr(null);
    setSubmitting(true);
    try {
      const specs: Record<string, Spec> = {};
      for (const id of selected) specs[id] = toSpec(spec);
      const req: StartRunRequest = { name: spec.name, specs };
      const { runId } = await api.startRun(req);
      nav(`/runs/${runId}`);
    } catch (e) {
      setErr(String(e));
      setSubmitting(false);
    }
  }

  const steps = ["Workload", "I/O & Last", "Ziel", "Server & Start"];

  return (
    <div style={{ maxWidth: 680 }}>
      <h1>Neuer Test</h1>
      <div className="inline" style={{ marginBottom: 20 }}>
        {steps.map((label, i) => (
          <span
            key={label}
            className="badge"
            style={{
              borderColor: i === step ? "var(--accent)" : undefined,
              color: i === step ? "var(--text)" : "var(--muted)",
            }}
          >
            {i + 1}. {label}
          </span>
        ))}
      </div>

      <div className="card">
        {step === 0 && (
          <>
            <div className="form-row">
              <label>Testname</label>
              <input
                value={spec.name}
                onChange={(e) => set("name", e.target.value)}
              />
            </div>
            <div className="form-row">
              <label>Workload</label>
              <select
                value={spec.workload}
                onChange={(e) => set("workload", e.target.value as Workload)}
              >
                {WORKLOADS.map((w) => (
                  <option key={w.value} value={w.value}>
                    {w.label}
                  </option>
                ))}
              </select>
            </div>
            {isMixed(spec.workload) && (
              <div className="form-row">
                <label>Lese-Anteil (%)</label>
                <input
                  type="number"
                  min={0}
                  max={100}
                  value={spec.rwMixRead}
                  onChange={(e) => set("rwMixRead", Number(e.target.value))}
                />
              </div>
            )}
          </>
        )}

        {step === 1 && (
          <>
            <div className="inline">
              <div className="form-row" style={{ flex: 1 }}>
                <label>Blockgröße</label>
                <select
                  value={spec.blockSize}
                  onChange={(e) => set("blockSize", e.target.value)}
                >
                  {BLOCK_SIZES.map((b) => (
                    <option key={b} value={b}>
                      {b}
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-row" style={{ flex: 1 }}>
                <label>I/O-Engine</label>
                <select
                  value={spec.engine}
                  onChange={(e) => set("engine", e.target.value as IOEngine)}
                >
                  {ENGINES.map((en) => (
                    <option key={en} value={en}>
                      {en}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div className="inline">
              <div className="form-row" style={{ flex: 1 }}>
                <label>IO-Depth</label>
                <input
                  type="number"
                  min={1}
                  value={spec.ioDepth}
                  onChange={(e) => set("ioDepth", Number(e.target.value))}
                />
              </div>
              <div className="form-row" style={{ flex: 1 }}>
                <label>Parallele Jobs</label>
                <input
                  type="number"
                  min={1}
                  value={spec.numJobs}
                  onChange={(e) => set("numJobs", Number(e.target.value))}
                />
              </div>
            </div>
            <div className="inline">
              <div className="form-row" style={{ flex: 1 }}>
                <label>Laufzeit (s)</label>
                <input
                  type="number"
                  min={1}
                  value={spec.runtimeSeconds}
                  onChange={(e) => set("runtimeSeconds", Number(e.target.value))}
                />
              </div>
              <div className="form-row" style={{ flex: 1 }}>
                <label>Warmlauf (s)</label>
                <input
                  type="number"
                  min={0}
                  value={spec.rampSeconds}
                  onChange={(e) => set("rampSeconds", Number(e.target.value))}
                />
              </div>
            </div>
            <div className="inline">
              <div className="form-row" style={{ flex: 1 }}>
                <label>IOPS-Limit (0 = unbegrenzt)</label>
                <input
                  type="number"
                  min={0}
                  value={spec.targetIOPS}
                  onChange={(e) => set("targetIOPS", Number(e.target.value))}
                />
              </div>
              <div className="form-row" style={{ flex: 1, justifyContent: "end" }}>
                <label>
                  <input
                    type="checkbox"
                    checked={spec.directIO}
                    onChange={(e) => set("directIO", e.target.checked)}
                  />{" "}
                  Direct I/O (O_DIRECT)
                </label>
              </div>
            </div>
          </>
        )}

        {step === 2 && (
          <>
            <div className="form-row">
              <label>Zieltyp</label>
              <select
                value={spec.targetKind}
                onChange={(e) => set("targetKind", e.target.value as TargetKind)}
              >
                <option value="file">Datei (Dateisystem)</option>
                <option value="device">Block-Device (/dev/...)</option>
              </select>
            </div>
            <div className="form-row">
              <label>Zielpfad</label>
              <input
                value={spec.targetPath}
                onChange={(e) => set("targetPath", e.target.value)}
                placeholder={
                  spec.targetKind === "device"
                    ? "/dev/mapper/..."
                    : "/var/tmp/storagebench.test"
                }
              />
            </div>
            {spec.targetKind === "file" && (
              <div className="form-row">
                <label>Dateigröße (MiB)</label>
                <input
                  type="number"
                  min={1}
                  value={spec.fileSizeMiB}
                  onChange={(e) => set("fileSizeMiB", Number(e.target.value))}
                />
              </div>
            )}
            {spec.targetKind === "device" && isWrite(spec.workload) && (
              <div className="form-row">
                <label className="error">
                  Schreib-Workload auf ein Block-Device ist destruktiv.
                </label>
                <label>
                  <input
                    type="checkbox"
                    checked={spec.allowDestructive}
                    onChange={(e) => set("allowDestructive", e.target.checked)}
                  />{" "}
                  Ja, Daten auf dem Zielgerät dürfen überschrieben werden
                  (allowDestructive)
                </label>
              </div>
            )}
          </>
        )}

        {step === 3 && (
          <>
            <h2>Server auswählen</h2>
            {agents.length === 0 && (
              <div className="muted">Keine Agenten verbunden.</div>
            )}
            {agents.map((a) => (
              <div className="form-row" key={a.agentId}>
                <label>
                  <input
                    type="checkbox"
                    checked={selected.has(a.agentId)}
                    onChange={() => toggleAgent(a.agentId)}
                  />{" "}
                  {a.system.hostname}{" "}
                  <span className="muted">
                    ({a.status}
                    {a.capabilities.fio ? "" : ", kein fio"})
                  </span>
                </label>
              </div>
            ))}
            {deviceWriteWithoutOptIn && (
              <div className="error">
                Destruktiver Test ohne Bestätigung – bitte in Schritt 3 den
                allowDestructive-Haken setzen.
              </div>
            )}
            {err && <div className="error">{err}</div>}
          </>
        )}
      </div>

      <div className="inline" style={{ marginTop: 16, justifyContent: "space-between" }}>
        <button
          className="btn secondary"
          disabled={step === 0}
          onClick={() => setStep((s) => Math.max(0, s - 1))}
        >
          Zurück
        </button>
        {step < steps.length - 1 ? (
          <button className="btn" onClick={() => setStep((s) => s + 1)}>
            Weiter
          </button>
        ) : (
          <button className="btn" disabled={!canSubmit || submitting} onClick={submit}>
            {submitting ? "Starte…" : "Test starten"}
          </button>
        )}
      </div>
    </div>
  );
}
