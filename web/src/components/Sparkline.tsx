// Sparkline: dependency-free SVG trend line that stretches to its container.

interface Props {
  values: number[];
  height?: number;
  color?: string;
}

export function Sparkline({ values, height = 32, color = "var(--accent)" }: Props) {
  if (values.length < 2) return null;
  const W = 100;
  const max = Math.max(...values, 1);
  const step = W / (values.length - 1);
  const pts = values.map((v, i) => [i * step, height - (v / max) * (height - 2) - 1]);
  const line = pts.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(1)}`).join(" ");
  const area = `0,${height} ${line} ${W},${height}`;
  return (
    <svg
      width="100%"
      height={height}
      viewBox={`0 0 ${W} ${height}`}
      preserveAspectRatio="none"
      aria-hidden
      style={{ display: "block" }}
    >
      <polygon points={area} fill={color} opacity={0.08} />
      <polyline
        points={line}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        vectorEffect="non-scaling-stroke"
        strokeLinejoin="round"
      />
    </svg>
  );
}
