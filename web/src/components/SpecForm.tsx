import type { ReactNode } from "react";
import type { IOEngine, Spec, TargetKind, Workload } from "../api/types";

export const WORKLOADS: { value: Workload; label: string }[] = [
  { value: "randread", label: "Random Read – zufällig lesen" },
  { value: "randwrite", label: "Random Write – zufällig schreiben" },
  { value: "randrw", label: "Random Mixed – zufällig gemischt" },
  { value: "read", label: "Seq. Read – sequenziell lesen" },
  { value: "write", label: "Seq. Write – sequenziell schreiben" },
  { value: "rw", label: "Seq. Mixed – sequenziell gemischt" },
];
export const BLOCK_SIZES = [
  "512", "1k", "4k", "8k", "16k", "32k", "64k", "128k", "256k", "512k", "1m", "2m", "4m",
];
export const ENGINES: IOEngine[] = ["libaio", "io_uring", "psync", "sync"];

export function defaultSpec(name = "Test"): Spec {
  return {
    name,
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
  };
}

// Built-in workload presets. Picking one fills the step form with sensible,
// well-known parameters as a starting point; the user can still tweak anything.
export interface Preset {
  id: string;
  label: string;
  hint: string;
  // Partial spec merged onto defaultSpec(); `name` becomes the step name.
  spec: Partial<Spec>;
}

export const PRESETS: Preset[] = [
  {
    id: "rand4k_r_qd32",
    label: "4K Random Read · QD32",
    hint: "IOPS-Test, zufälliges Lesen – klassischer SSD-Durchsatz",
    spec: { name: "4K Rand Read QD32", workload: "randread", blockSize: "4k", ioDepth: 32, numJobs: 4 },
  },
  {
    id: "rand4k_w_qd32",
    label: "4K Random Write · QD32",
    hint: "IOPS-Test, zufälliges Schreiben",
    spec: { name: "4K Rand Write QD32", workload: "randwrite", blockSize: "4k", ioDepth: 32, numJobs: 4 },
  },
  {
    id: "oltp_70_30",
    label: "OLTP 70/30 · 8K Random",
    hint: "Datenbank-ähnlich: 70 % lesen, 30 % schreiben",
    spec: { name: "OLTP 70/30 8K", workload: "randrw", blockSize: "8k", rwMixRead: 70, ioDepth: 32, numJobs: 4 },
  },
  {
    id: "seq_read_1m",
    label: "Sequenziell Lesen · 1M",
    hint: "Bandbreiten-Test, großes Blocksize, QD niedrig",
    spec: { name: "Seq Read 1M", workload: "read", blockSize: "1m", ioDepth: 8, numJobs: 1 },
  },
  {
    id: "seq_write_1m",
    label: "Sequenziell Schreiben · 1M",
    hint: "Bandbreiten-Test schreibend",
    spec: { name: "Seq Write 1M", workload: "write", blockSize: "1m", ioDepth: 8, numJobs: 1 },
  },
  {
    id: "lat_qd1",
    label: "Latenz · 4K QD1",
    hint: "Reine Latenzmessung, eine ausstehende I/O",
    spec: { name: "Latenz 4K QD1", workload: "randread", blockSize: "4k", ioDepth: 1, numJobs: 1 },
  },
  {
    id: "fsync_heavy",
    label: "fsync-lastig · 4K sync",
    hint: "Journaling/Commit-Last: sync-Engine, kleine Blöcke",
    spec: { name: "fsync 4K", workload: "randwrite", blockSize: "4k", engine: "sync", ioDepth: 1, numJobs: 4, directIO: false },
  },
];

// applyPreset merges a preset onto the current spec, keeping the target
// (path/kind/size) the user already set.
export function applyPreset(current: Spec, p: Preset): Spec {
  return {
    ...current,
    ...p.spec,
    name: p.spec.name ?? current.name,
    targetKind: current.targetKind,
    targetPath: current.targetPath,
    fileSizeMiB: current.fileSizeMiB,
  };
}

export const isMixed = (w: Workload) => w === "randrw" || w === "rw";
export const isWrite = (w: Workload) =>
  w === "randwrite" || w === "write" || w === "randrw" || w === "rw";

export type SpecErrors = Partial<Record<keyof Spec, string>>;

// validateSpec returns blocking errors per field. Kept next to the form so the
// editor can mark invalid steps in the step list as well.
export function validateSpec(s: Spec): SpecErrors {
  const e: SpecErrors = {};
  if (!s.name.trim()) e.name = "Name fehlt";
  if (!s.targetPath.trim()) e.targetPath = "Zielpfad fehlt";
  else if (!s.targetPath.startsWith("/")) e.targetPath = "Absoluter Pfad erwartet (beginnt mit /)";
  else if (s.targetKind === "device" && !s.targetPath.startsWith("/dev/"))
    e.targetPath = "Block-Device liegt unter /dev/";
  if (!(s.ioDepth >= 1)) e.ioDepth = "mind. 1";
  if (!(s.numJobs >= 1)) e.numJobs = "mind. 1";
  if (!(s.runtimeSeconds >= 1)) e.runtimeSeconds = "mind. 1 s";
  if ((s.rampSeconds ?? 0) < 0) e.rampSeconds = "nicht negativ";
  if (s.targetKind === "file" && !((s.fileSizeMiB ?? 0) >= 1)) e.fileSizeMiB = "mind. 1 MiB";
  if (isMixed(s.workload)) {
    const r = s.rwMixRead ?? 70;
    if (r < 0 || r > 100) e.rwMixRead = "0 bis 100";
  }
  if ((s.targetIOPS ?? 0) < 0) e.targetIOPS = "nicht negativ";
  return e;
}

// Destructive device write without explicit consent: allowed to save, but the
// controller will reject the step at run time.
export const needsConsent = (s: Spec) =>
  s.targetKind === "device" && isWrite(s.workload) && !s.allowDestructive;

interface Props {
  spec: Spec;
  onChange: (s: Spec) => void;
  errors?: SpecErrors;
}

function Field({
  label,
  error,
  hint,
  className = "",
  children,
}: {
  label: string;
  error?: string;
  hint?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={`field ${error ? "invalid" : ""} ${className}`}>
      <label>{label}</label>
      {children}
      {error ? <span className="field-err">{error}</span> : hint ? <span className="hint">{hint}</span> : null}
    </div>
  );
}

function Num({
  value,
  onChange,
  min,
  max,
  unit,
}: {
  value: number;
  onChange: (n: number) => void;
  min?: number;
  max?: number;
  unit?: string;
}) {
  const input = (
    <input
      type="number"
      min={min}
      max={max}
      value={Number.isFinite(value) ? value : ""}
      onChange={(e) => onChange(e.target.value === "" ? NaN : Number(e.target.value))}
    />
  );
  return unit ? (
    <div className="input-unit">
      {input}
      <span>{unit}</span>
    </div>
  ) : (
    input
  );
}

// SpecForm edits a single benchmark step, grouped by what the parameters mean:
// load pattern, concurrency, duration, target.
export function SpecForm({ spec, onChange, errors = {} }: Props) {
  const set = <K extends keyof Spec>(k: K, v: Spec[K]) => onChange({ ...spec, [k]: v });

  function onPreset(id: string) {
    const p = PRESETS.find((x) => x.id === id);
    if (p) onChange(applyPreset(spec, p));
  }

  return (
    <div>
      <div className="form-section">
        <div className="form-grid">
          <Field label="Name des Schritts" error={errors.name} className="span-2">
            <input value={spec.name} onChange={(e) => set("name", e.target.value)} />
          </Field>
          <Field label="Vorlage anwenden" hint="Überschreibt Last- und Parallelitätswerte, Ziel bleibt" className="span-2">
            <select value="" onChange={(e) => onPreset(e.target.value)}>
              <option value="" disabled>
                Vorlage wählen…
              </option>
              {PRESETS.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.label} – {p.hint}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </div>

      <div className="form-section">
        <h3>
          Lastmuster <small>Was gemessen wird</small>
        </h3>
        <div className="form-grid">
          <Field label="Workload" className="span-2">
            <select value={spec.workload} onChange={(e) => set("workload", e.target.value as Workload)}>
              {WORKLOADS.map((w) => (
                <option key={w.value} value={w.value}>
                  {w.label}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Blockgröße">
            <select value={spec.blockSize} onChange={(e) => set("blockSize", e.target.value)}>
              {BLOCK_SIZES.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </Field>
          {isMixed(spec.workload) && (
            <Field label="Lese-Anteil" error={errors.rwMixRead} hint={`${100 - (spec.rwMixRead ?? 70)} % schreiben`}>
              <Num value={spec.rwMixRead ?? 70} min={0} max={100} unit="%" onChange={(n) => set("rwMixRead", n)} />
            </Field>
          )}
        </div>
      </div>

      <div className="form-section">
        <h3>
          Parallelität &amp; Dauer <small>Wie stark und wie lange belastet wird</small>
        </h3>
        <div className="form-grid">
          <Field label="IO-Depth" error={errors.ioDepth} hint="ausstehende I/Os je Job">
            <Num value={spec.ioDepth} min={1} onChange={(n) => set("ioDepth", n)} />
          </Field>
          <Field label="Parallele Jobs" error={errors.numJobs}>
            <Num value={spec.numJobs} min={1} onChange={(n) => set("numJobs", n)} />
          </Field>
          <Field label="Laufzeit" error={errors.runtimeSeconds}>
            <Num value={spec.runtimeSeconds} min={1} unit="s" onChange={(n) => set("runtimeSeconds", n)} />
          </Field>
          <Field label="Warmlauf" error={errors.rampSeconds} hint="wird nicht gewertet">
            <Num value={spec.rampSeconds ?? 0} min={0} unit="s" onChange={(n) => set("rampSeconds", n)} />
          </Field>
          <Field label="I/O-Engine">
            <select value={spec.engine} onChange={(e) => set("engine", e.target.value as IOEngine)}>
              {ENGINES.map((en) => (
                <option key={en} value={en}>
                  {en}
                </option>
              ))}
            </select>
          </Field>
          <Field label="IOPS-Limit" error={errors.targetIOPS} hint="0 = unbegrenzt">
            <Num value={spec.targetIOPS ?? 0} min={0} onChange={(n) => set("targetIOPS", n)} />
          </Field>
          <div className="field span-2" style={{ justifyContent: "center", paddingTop: 18 }}>
            <label className="check">
              <input type="checkbox" checked={spec.directIO} onChange={(e) => set("directIO", e.target.checked)} />
              <span>
                Direct I/O (O_DIRECT)
                <span className="desc">Page-Cache umgehen, misst das Speichersystem statt RAM</span>
              </span>
            </label>
          </div>
        </div>
      </div>

      <div className="form-section">
        <h3>
          Ziel <small>Wohin geschrieben bzw. woraus gelesen wird</small>
        </h3>
        <div className="form-grid">
          <Field label="Zieltyp">
            <select value={spec.targetKind} onChange={(e) => set("targetKind", e.target.value as TargetKind)}>
              <option value="file">Datei im Dateisystem</option>
              <option value="device">Block-Device</option>
            </select>
          </Field>
          <Field label="Zielpfad" error={errors.targetPath} className="span-2">
            <input
              className="mono-input"
              value={spec.targetPath}
              onChange={(e) => set("targetPath", e.target.value)}
              placeholder={spec.targetKind === "device" ? "/dev/mapper/…" : "/var/tmp/storagebench.test"}
            />
          </Field>
          {spec.targetKind === "file" && (
            <Field label="Dateigröße" error={errors.fileSizeMiB}>
              <Num value={spec.fileSizeMiB ?? 1024} min={1} unit="MiB" onChange={(n) => set("fileSizeMiB", n)} />
            </Field>
          )}
        </div>

        {spec.targetKind === "device" && isWrite(spec.workload) && (
          <div className={`callout ${spec.allowDestructive ? "warn" : "err"}`} style={{ marginTop: 14 }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
              <path d="M12 9v4M12 17h.01M10.3 3.9L2 18a2 2 0 0 0 1.7 3h16.6a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
            </svg>
            <div>
              <div className="title">Schreibender Test auf ein Block-Device</div>
              <div className="body" style={{ marginBottom: 8 }}>
                Alle Daten auf {spec.targetPath || "dem Gerät"} werden überschrieben. Ohne Bestätigung lehnt der
                Controller diesen Schritt beim Ausführen ab.
              </div>
              <label className="check">
                <input
                  type="checkbox"
                  checked={spec.allowDestructive ?? false}
                  onChange={(e) => set("allowDestructive", e.target.checked)}
                />
                Ich bestätige: Daten auf dem Zielgerät dürfen überschrieben werden
              </label>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
