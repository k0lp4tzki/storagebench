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

  return (
    <div>
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
