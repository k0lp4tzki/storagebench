import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import { useTheme } from "../state/ThemeProvider";

export interface ChartSeries {
  label: string;
  // Y values aligned to the shared x array (seconds-from-start or timestamps).
  data: number[];
  color: string;
}

interface Props {
  // X axis values (seconds from start). Shared by all series.
  x: number[];
  series: ChartSeries[];
  height?: number;
  // Formatter for the y-axis / legend values.
  yFormat?: (v: number) => string;
  title?: string;
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

// Chart is a theme-aware uPlot time-series chart with axes, grid, legend and a
// hover cursor. It resizes with its container and rebuilds on theme change.
export function Chart({ x, series, height = 200, yFormat, title }: Props) {
  const { theme } = useTheme();
  const wrapRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | null>(null);
  const roRef = useRef<ResizeObserver | null>(null);

  // (Re)build the plot when structure, theme, or formatter identity changes.
  useEffect(() => {
    const wrap = wrapRef.current;
    if (!wrap) return;

    const gridColor = cssVar("--border") || "#e3e6ea";
    const textColor = cssVar("--text-muted") || "#6b7280";

    const fmt = yFormat ?? ((v: number) => String(Math.round(v)));

    const opts: uPlot.Options = {
      title,
      width: wrap.clientWidth || 600,
      height,
      padding: [12, 12, 4, 8],
      cursor: {
        y: false,
        points: { size: 6 },
      },
      legend: { show: true, live: true },
      scales: {
        x: { time: false },
        y: { range: (_u, min, max) => [Math.min(0, min), max <= 0 ? 1 : max * 1.08] },
      },
      axes: [
        {
          stroke: textColor,
          grid: { stroke: gridColor, width: 1 },
          ticks: { stroke: gridColor, width: 1 },
          values: (_u, vals) => vals.map((v) => `${v}s`),
          font: "11px Inter, sans-serif",
        },
        {
          stroke: textColor,
          grid: { stroke: gridColor, width: 1 },
          ticks: { stroke: gridColor, width: 1 },
          size: 56,
          values: (_u, vals) => vals.map((v) => fmt(v)),
          font: "11px Inter, sans-serif",
        },
      ],
      series: [
        { label: "t", value: (_u, v) => (v == null ? "–" : `${v}s`) },
        ...series.map((s) => ({
          label: s.label,
          stroke: s.color,
          width: 2,
          points: { show: false },
          value: (_u: uPlot, v: number | null) => (v == null ? "–" : fmt(v)),
        })),
      ],
    };

    const data: uPlot.AlignedData = [
      x,
      ...series.map((s) => s.data),
    ];

    const plot = new uPlot(opts, data, wrap);
    plotRef.current = plot;

    const ro = new ResizeObserver(() => {
      plot.setSize({ width: wrap.clientWidth || 600, height });
    });
    ro.observe(wrap);
    roRef.current = ro;

    return () => {
      ro.disconnect();
      plot.destroy();
      plotRef.current = null;
    };
    // Rebuild on theme (colors), series count/labels, height, title.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [theme, height, title, series.map((s) => s.label + s.color).join("|")]);

  // Fast path: update data without rebuilding when only values change.
  useEffect(() => {
    const plot = plotRef.current;
    if (!plot) return;
    const data: uPlot.AlignedData = [x, ...series.map((s) => s.data)];
    plot.setData(data);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [x, series]);

  return <div ref={wrapRef} style={{ width: "100%" }} />;
}
