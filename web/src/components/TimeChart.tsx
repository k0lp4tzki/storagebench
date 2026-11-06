// TimeChart renders one or more numeric series on a shared Y scale as an SVG.
// Dependency-free, good enough for live IOPS/latency views without pulling in a
// charting library.

export interface Series {
  values: number[];
  color: string;
  label: string;
}

interface Props {
  series: Series[];
  width?: number;
  height?: number;
  unit?: string;
}

export function TimeChart({ series, width = 420, height = 160, unit }: Props) {
  const all = series.flatMap((s) => s.values);
  const max = all.length ? Math.max(...all, 0) : 1;
  const padding = { top: 8, right: 8, bottom: 8, left: 44 };
  const plotW = width - padding.left - padding.right;
  const plotH = height - padding.top - padding.bottom;
  const scaleY = (v: number) => padding.top + plotH - (v / (max || 1)) * plotH;

  const maxLen = Math.max(1, ...series.map((s) => s.values.length));
  const scaleX = (i: number) =>
    padding.left + (maxLen <= 1 ? 0 : (i / (maxLen - 1)) * plotW);

  return (
    <div>
      <svg width={width} height={height}>
        {/* baseline */}
        <line
          x1={padding.left}
          y1={padding.top + plotH}
          x2={width - padding.right}
          y2={padding.top + plotH}
          stroke="var(--border)"
        />
        <text x={4} y={padding.top + 10} fill="var(--muted)" fontSize={10}>
          {formatAxis(max)}
          {unit ? ` ${unit}` : ""}
        </text>
        {series.map((s) =>
          s.values.length >= 2 ? (
            <polyline
              key={s.label}
              fill="none"
              stroke={s.color}
              strokeWidth={1.5}
              points={s.values
                .map((v, i) => `${scaleX(i).toFixed(1)},${scaleY(v).toFixed(1)}`)
                .join(" ")}
            />
          ) : null,
        )}
      </svg>
      <div className="inline" style={{ gap: 14, marginTop: 4 }}>
        {series.map((s) => (
          <span key={s.label} style={{ fontSize: 11, color: "var(--muted)" }}>
            <span
              style={{
                display: "inline-block",
                width: 10,
                height: 2,
                background: s.color,
                marginRight: 4,
                verticalAlign: "middle",
              }}
            />
            {s.label}
          </span>
        ))}
      </div>
    </div>
  );
}

function formatAxis(v: number): string {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000) return `${(v / 1_000).toFixed(1)}k`;
  return v.toFixed(0);
}
