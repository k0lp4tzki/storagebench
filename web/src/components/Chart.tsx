import { useEffect, useMemo, useRef, useState } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import { useTheme } from "../state/ThemeProvider";

export interface ChartSeries {
  label: string;
  // Y values aligned to the shared x array (seconds from start).
  data: number[];
  // A literal colour or a CSS custom property reference like "var(--read)".
  color: string;
}

// ChartMarker is a vertical annotation at an x position (seconds from start),
// used to show fault inject/revert moments on the timeline.
export interface ChartMarker {
  x: number;
  label: string;
  // "inject" (warning colour) or "revert" (muted/ok colour).
  kind: "inject" | "revert";
}

interface Props {
  x: number[];
  series: ChartSeries[];
  height?: number;
  yFormat?: (v: number) => string;
  title?: string;
  // Text shown over the empty plot when there is no data yet.
  emptyText?: string;
  // Fill the area under each line (good for 1–2 series, noisy for many).
  fill?: boolean;
  // Optional vertical markers (e.g. fault inject/revert) drawn over the plot.
  markers?: ChartMarker[];
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

// Canvas cannot resolve CSS variables, so "var(--x)" must be looked up first.
function resolveColor(c: string): string {
  const m = /^var\((--[\w-]+)\)$/.exec(c.trim());
  return m ? cssVar(m[1]) || "#888" : c;
}

function withAlpha(hex: string, a: number): string {
  if (/^#[0-9a-f]{6}$/i.test(hex)) {
    return hex + Math.round(a * 255).toString(16).padStart(2, "0");
  }
  return hex;
}

// "Nice" integer increments: avoids duplicate tick labels such as 0 / 1 / 1
// when the range is tiny and values get rounded by the formatter.
const Y_INCRS: number[] = [];
for (let e = 0; e <= 9; e++) {
  for (const m of [1, 2, 2.5, 5]) {
    const v = m * 10 ** e;
    if (Number.isInteger(v)) Y_INCRS.push(v);
  }
}
const X_INCRS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200];

function yMaxOf(cols: (number | null)[][]): number {
  let m = 0;
  for (const col of cols) {
    for (const v of col) {
      if (typeof v === "number" && Number.isFinite(v) && v > m) m = v;
    }
  }
  return m;
}

function yRange(max: number): [number, number] {
  return [0, max > 0 ? max * 1.12 : 10];
}

function fmtSeconds(v: number): string {
  if (v < 120) return `${v}s`;
  const m = Math.floor(v / 60);
  const s = v % 60;
  return s ? `${m}:${String(s).padStart(2, "0")}` : `${m}m`;
}

function paint(plot: uPlot, x: number[], cols: (number | null)[][]): void {
  plot.setData([x, ...cols], true);
  const [min, max] = yRange(yMaxOf(cols));
  plot.setScale("y", { min, max });
  // setScale updates axes but not series paths; toggling the theme used to
  // "fix" that by destroying the plot. Rebuild paths in place instead.
  plot.redraw(true, false);
}

// Chart: theme-aware uPlot time series with a compact header legend that shows
// the latest values, or the hovered values while the cursor is over the plot.
export function Chart({
  x,
  series,
  height = 180,
  yFormat,
  title,
  emptyText = "Noch keine Messwerte",
  fill = true,
  markers,
}: Props) {
  const { theme } = useTheme();
  const wrapRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | null>(null);
  // Align every series to the x length so uPlot never silently drops a series
  // (and thus mis-computes the y-range) when arrays are momentarily mismatched.
  const alignedData = useMemo<(number | null)[][]>(() => {
    const n = x.length;
    return series.map((s) => {
      if (s.data.length === n) return s.data;
      const out: (number | null)[] = new Array(n).fill(null);
      for (let i = 0; i < Math.min(n, s.data.length); i++) out[i] = s.data[i];
      return out;
    });
  }, [x, series]);
  // Keep the latest markers in a ref so the draw hook can read them without
  // rebuilding the plot on every marker change.
  const markersRef = useRef<ChartMarker[] | undefined>(markers);
  markersRef.current = markers;
  const [hoverIdx, setHoverIdx] = useState<number | null>(null);
  const fmt = useMemo(() => yFormat ?? ((v: number) => String(Math.round(v))), [yFormat]);
  const colors = useMemo(
    () => series.map((s) => resolveColor(s.color)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [theme, series.map((s) => s.color).join("|")],
  );
  const yMaxRef = useRef(0);
  yMaxRef.current = yMaxOf(alignedData);
  const xRef = useRef(x);
  const dataRef = useRef(alignedData);
  xRef.current = x;
  dataRef.current = alignedData;

  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;
    const grid = cssVar("--border") || "#2a2f37";
    const text = cssVar("--text-faint") || "#7b838f";
    const font = "11px ui-monospace, Menlo, Consolas, monospace";
    let plot: uPlot | null = null;
    let dead = false;

    const optsFor = (width: number): uPlot.Options => ({
      width,
      height,
      padding: [8, 8, 0, 0],
      legend: { show: false },
      cursor: {
        y: false,
        points: { size: 7, width: 2 },
        drag: { x: false, y: false },
      },
      scales: {
        x: { time: false },
        y: {
          range: () => yRange(yMaxRef.current),
        },
      },
      axes: [
        {
          stroke: text,
          font,
          grid: { show: false },
          ticks: { stroke: grid, width: 1, size: 4 },
          incrs: X_INCRS,
          values: (_u, vals) => vals.map((v) => fmtSeconds(v)),
          space: 60,
          size: 28,
        },
        {
          stroke: text,
          font,
          grid: { stroke: grid, width: 1 },
          ticks: { show: false },
          incrs: Y_INCRS,
          size: 58,
          space: 32,
          values: (_u, vals) => vals.map((v) => fmt(v)),
        },
      ],
      series: [
        {},
        ...series.map((s, i) => ({
          label: s.label,
          stroke: colors[i],
          width: 1.75,
          fill: fill && series.length <= 2 ? withAlpha(colors[i], 0.1) : undefined,
          points: { show: false },
        })),
      ],
      hooks: {
        setCursor: [
          (u: uPlot) => {
            const idx = u.cursor.idx;
            setHoverIdx(idx == null || u.cursor.left == null || u.cursor.left < 0 ? null : idx);
          },
        ],
        draw: [
          (u: uPlot) => {
            const ms = markersRef.current;
            if (!ms || ms.length === 0) return;
            const ctx = u.ctx;
            const warn = cssVar("--warn") || "#e8b339";
            const ok = cssVar("--text-faint") || "#7b838f";
            ctx.save();
            ctx.lineWidth = 1.5;
            ctx.setLineDash([4, 3]);
            for (const m of ms) {
              const xpos = u.valToPos(m.x, "x", true);
              if (xpos < u.bbox.left || xpos > u.bbox.left + u.bbox.width) continue;
              ctx.strokeStyle = m.kind === "inject" ? warn : ok;
              ctx.beginPath();
              ctx.moveTo(xpos, u.bbox.top);
              ctx.lineTo(xpos, u.bbox.top + u.bbox.height);
              ctx.stroke();
            }
            ctx.restore();
          },
        ],
      },
    });

    const ensure = () => {
      if (dead) return;
      const w = wrap.clientWidth;
      if (w < 32) return;
      if (!plot) {
        plot = new uPlot(optsFor(w), [xRef.current, ...dataRef.current], wrap);
        plotRef.current = plot;
        paint(plot, xRef.current, dataRef.current);
        return;
      }
      plot.setSize({ width: w, height });
      plot.redraw(true, false);
    };

    const ro = new ResizeObserver(ensure);
    ro.observe(wrap);
    ensure();
    return () => {
      dead = true;
      ro.disconnect();
      plot?.destroy();
      plotRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [theme, height, fill, colors.join("|"), series.map((s) => s.label).join("|"), fmt]);

  useEffect(() => {
    const plot = plotRef.current;
    if (!plot) return;
    paint(plot, x, alignedData);
  }, [x, alignedData]);

  // Redraw when markers change (the draw hook reads them from the ref).
  useEffect(() => {
    plotRef.current?.redraw();
  }, [markers]);

  const n = x.length;
  const idx = hoverIdx != null && hoverIdx < n ? hoverIdx : n - 1;

  return (
    <div>
      <div className="chart-head">
        {title && <span className="t">{title}</span>}
        {series.map((s, i) => {
          const v = idx >= 0 ? s.data[idx] : undefined;
          return (
            <span className="legend" key={s.label}>
              <i style={{ background: colors[i] }} />
              {s.label}
              <b>{v == null ? "–" : fmt(v)}</b>
            </span>
          );
        })}
        {hoverIdx != null && n > 0 && (
          <span className="faint mono" style={{ marginLeft: "auto" }}>
            t = {fmtSeconds(x[idx])}
          </span>
        )}
      </div>
      <div className="chart-box">
        <div ref={wrapRef} style={{ width: "100%" }} />
        {n === 0 && <div className="chart-empty">{emptyText}</div>}
      </div>
    </div>
  );
}
