import type { IOEngine, Spec, TargetKind, Workload } from "../api/types";

export const WORKLOADS: { value: Workload; label: string }[] = [
  { value: "randread", label: "Zufällig Lesen" },
  { value: "randwrite", label: "Zufällig Schreiben" },
  { value: "randrw", label: "Zufällig gemischt" },
  { value: "read", label: "Sequenziell Lesen" },
  { value: "write", label: "Sequenziell Schreiben" },
  { value: "rw", label: "Sequenziell gemischt" },
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

interface Props {
  spec: Spec;
  onChange: (s: Spec) => void;
}

// SpecForm edits a single benchmark spec. Used inside the profile step editor.
export function SpecForm({ spec, onChange }: Props) {
  const set = <K extends keyof Spec>(k: K, v: Spec[K]) =>
    onChange({ ...spec, [k]: v });

  const deviceWrite =
    spec.targetKind === "device" && isWrite(spec.workload) && !spec.allowDestructive;

  function onPreset(id: string) {
    const p = PRESETS.find((x) => x.id === id);
    if (p) onChange(applyPreset(spec, p));
  }

  return (
    <div>
      <div className="field">
        <label>Vorlage (füllt die Felder aus)</label>
        <select value="" onChange={(e) => onPreset(e.target.value)}>
          <option value="" disabled>
            Vorlage wählen…
          </option>
          {PRESETS.map((p) => (
            <option key={p.id} value={p.id}>
              {p.label} — {p.hint}
            </option>
          ))}
        </select>
      </div>

      <div className="field">
        <label>Name des Schritts</label>
        <input value={spec.name} onChange={(e) => set("name", e.target.value)} />
      </div>

      <div className="row">
        <div className="field">
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
        <div className="field">
          <label>Blockgröße</label>
          <select value={spec.blockSize} onChange={(e) => set("blockSize", e.target.value)}>
            {BLOCK_SIZES.map((b) => (
              <option key={b} value={b}>
                {b}
              </option>
            ))}
          </select>
        </div>
        <div className="field">
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

      {isMixed(spec.workload) && (
        <div className="field">
          <label>Lese-Anteil (%)</label>
          <input
            type="number"
            min={0}
            max={100}
            value={spec.rwMixRead ?? 70}
            onChange={(e) => set("rwMixRead", Number(e.target.value))}
          />
        </div>
      )}

      <div className="row">
        <div className="field">
          <label>IO-Depth</label>
          <input
            type="number"
            min={1}
            value={spec.ioDepth}
            onChange={(e) => set("ioDepth", Number(e.target.value))}
          />
        </div>
        <div className="field">
          <label>Parallele Jobs</label>
          <input
            type="number"
            min={1}
            value={spec.numJobs}
            onChange={(e) => set("numJobs", Number(e.target.value))}
          />
        </div>
        <div className="field">
          <label>Laufzeit (s)</label>
          <input
            type="number"
            min={1}
            value={spec.runtimeSeconds}
            onChange={(e) => set("runtimeSeconds", Number(e.target.value))}
          />
        </div>
        <div className="field">
          <label>Warmlauf (s)</label>
          <input
            type="number"
            min={0}
            value={spec.rampSeconds ?? 0}
            onChange={(e) => set("rampSeconds", Number(e.target.value))}
          />
        </div>
      </div>

      <div className="row">
        <div className="field">
          <label>Zieltyp</label>
          <select
            value={spec.targetKind}
            onChange={(e) => set("targetKind", e.target.value as TargetKind)}
          >
            <option value="file">Datei (Dateisystem)</option>
            <option value="device">Block-Device (/dev/...)</option>
          </select>
        </div>
        <div className="field" style={{ flex: 2 }}>
          <label>Zielpfad</label>
          <input
            value={spec.targetPath}
            onChange={(e) => set("targetPath", e.target.value)}
            placeholder={
              spec.targetKind === "device" ? "/dev/mapper/..." : "/var/tmp/storagebench.test"
            }
          />
        </div>
        {spec.targetKind === "file" && (
          <div className="field">
            <label>Dateigröße (MiB)</label>
            <input
              type="number"
              min={1}
              value={spec.fileSizeMiB ?? 1024}
              onChange={(e) => set("fileSizeMiB", Number(e.target.value))}
            />
          </div>
        )}
      </div>

      <div className="row" style={{ alignItems: "center" }}>
        <label className="checkbox">
          <input
            type="checkbox"
            checked={spec.directIO}
            onChange={(e) => set("directIO", e.target.checked)}
          />
          Direct I/O (O_DIRECT)
        </label>
        <div className="field" style={{ marginBottom: 0 }}>
          <label>IOPS-Limit (0 = unbegrenzt)</label>
          <input
            type="number"
            min={0}
            value={spec.targetIOPS ?? 0}
            onChange={(e) => set("targetIOPS", Number(e.target.value))}
          />
        </div>
      </div>

      {spec.targetKind === "device" && isWrite(spec.workload) && (
        <div
          style={{
            marginTop: 8,
            padding: 12,
            borderRadius: 8,
            background: "var(--err-weak)",
          }}
        >
          <label className="checkbox">
            <input
              type="checkbox"
              checked={spec.allowDestructive ?? false}
              onChange={(e) => set("allowDestructive", e.target.checked)}
            />
            <span className="error" style={{ background: "transparent", padding: 0 }}>
              Destruktiv: Daten auf dem Zielgerät dürfen überschrieben werden
            </span>
          </label>
          {deviceWrite && (
            <div className="faint" style={{ fontSize: 12, marginTop: 6 }}>
              Ohne Bestätigung wird dieser Schritt beim Ausführen abgelehnt.
            </div>
          )}
        </div>
      )}
    </div>
  );
}
