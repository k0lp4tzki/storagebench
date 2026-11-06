// Small formatting helpers shared across pages.

export function fmtIOPS(v: number | undefined): string {
  if (v == null) return "–";
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(2)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`;
  return v.toFixed(0);
}

// Bandwidth values from fio are in KiB/s.
export function fmtBW(kibPerSec: number | undefined): string {
  if (kibPerSec == null) return "–";
  const mib = kibPerSec / 1024;
  if (mib >= 1024) return `${(mib / 1024).toFixed(2)} GiB/s`;
  if (mib >= 1) return `${mib.toFixed(1)} MiB/s`;
  return `${kibPerSec.toFixed(0)} KiB/s`;
}

export function fmtLatUs(us: number | undefined): string {
  if (us == null) return "–";
  if (us >= 1000) return `${(us / 1000).toFixed(2)} ms`;
  return `${us.toFixed(0)} µs`;
}

export function fmtTime(iso: string | undefined): string {
  if (!iso) return "–";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString("de-DE");
}

// secondsFromStart converts an array of wall-clock-millis timestamps into
// seconds elapsed from the first sample, for use as a chart x-axis.
export function secondsFromStart(tsMillis: number[]): number[] {
  if (tsMillis.length === 0) return [];
  const t0 = tsMillis[0];
  return tsMillis.map((t) => Math.max(0, Math.round((t - t0) / 1000)));
}

// coefficientOfVariation returns the relative standard deviation (stddev/mean)
// of a series as a fraction. It is a simple, unit-free stability measure: lower
// is steadier. Returns null when there is too little signal to be meaningful.
export function coefficientOfVariation(values: number[]): number | null {
  const xs = values.filter((v) => Number.isFinite(v));
  if (xs.length < 3) return null;
  const mean = xs.reduce((a, b) => a + b, 0) / xs.length;
  if (mean <= 0) return null;
  const variance =
    xs.reduce((a, b) => a + (b - mean) * (b - mean), 0) / xs.length;
  return Math.sqrt(variance) / mean;
}

// fmtPct formats a fraction (0..1) as a percentage string.
export function fmtPct(frac: number | null | undefined): string {
  if (frac == null || !Number.isFinite(frac)) return "–";
  return `${(frac * 100).toFixed(1)} %`;
}
