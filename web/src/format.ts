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

// ---- Time & duration -------------------------------------------------------

// parseTime returns epoch millis for an ISO string, or null for empty / Go
// zero-time values.
export function parseTime(iso: string | undefined | null): number | null {
  if (!iso) return null;
  const t = new Date(iso).getTime();
  if (isNaN(t) || t < 946684800000) return null; // before 2000 = unset
  return t;
}

export function fmtDateTime(iso: string | undefined): string {
  const t = parseTime(iso);
  if (t == null) return "–";
  return new Date(t).toLocaleString("de-DE", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function fmtClock(iso: string | undefined): string {
  const t = parseTime(iso);
  if (t == null) return "–";
  return new Date(t).toLocaleTimeString("de-DE", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

// fmtRelative renders "vor 5 Min." style labels for recent timestamps and
// falls back to a date for anything older than a week.
export function fmtRelative(iso: string | undefined, now = Date.now()): string {
  const t = parseTime(iso);
  if (t == null) return "–";
  const s = Math.round((now - t) / 1000);
  if (s < 45) return "gerade eben";
  const m = Math.round(s / 60);
  if (m < 60) return `vor ${m} Min.`;
  const h = Math.round(m / 60);
  if (h < 24) return `vor ${h} Std.`;
  const d = Math.round(h / 24);
  if (d === 1) return "gestern";
  if (d < 7) return `vor ${d} Tagen`;
  return new Date(t).toLocaleDateString("de-DE");
}

// fmtDuration formats seconds as "45 s", "3:20 min" or "1:05 h".
export function fmtDuration(sec: number | null | undefined): string {
  if (sec == null || !Number.isFinite(sec) || sec < 0) return "–";
  sec = Math.round(sec);
  if (sec < 60) return `${sec} s`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m < 60) return `${m}:${String(s).padStart(2, "0")} min`;
  const h = Math.floor(m / 60);
  return `${h}:${String(m % 60).padStart(2, "0")} h`;
}

// elapsedSec returns the duration between start and end (or now when running).
export function elapsedSec(
  start: string | undefined,
  end: string | undefined,
  running = false,
): number | null {
  const a = parseTime(start);
  if (a == null) return null;
  const b = parseTime(end) ?? (running ? Date.now() : null);
  if (b == null) return null;
  return Math.max(0, (b - a) / 1000);
}

export function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}
