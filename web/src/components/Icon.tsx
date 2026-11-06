// Small inline icon set (24px grid, 1.75 stroke). Inline SVG keeps the UI
// self-contained: the controller is often reachable only inside a lab network.

const PATHS: Record<string, string> = {
  dashboard: "M4 4h6v7H4zM14 4h6v4h-6zM14 12h6v8h-6zM4 15h6v5H4z",
  profiles: "M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01",
  live: "M3 12h4l3-8 4 16 3-8h4",
  results: "M4 20V10M10 20V4M16 20v-7M21 20H3",
  play: "M7 4.5v15l12-7.5z",
  plus: "M12 5v14M5 12h14",
  edit: "M4 20h4L19 9l-4-4L4 16zM13.5 6.5l4 4",
  trash: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6",
  copy: "M9 9h11v11H9zM5 15H4V4h11v1",
  up: "M6 15l6-6 6 6",
  down: "M6 9l6 6 6-6",
  right: "M9 6l6 6-6 6",
  x: "M6 6l12 12M18 6L6 18",
  sun: "M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4",
  moon: "M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z",
  arrow: "M5 12h14M13 6l6 6-6 6",
  stop: "M7 7h10v10H7z",
  alert: "M12 9v4M12 17h.01M10.3 3.9L2 18a2 2 0 0 0 1.7 3h16.6a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z",
  info: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 11v5M12 8h.01",
  clock: "M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18zM12 7v5l3 2",
  check: "M5 12.5l4.5 4.5L19 7",
  search: "M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM20 20l-4-4",
  server: "M4 4h16v7H4zM4 13h16v7H4zM8 7.5h.01M8 16.5h.01",
  restart: "M4 12a8 8 0 1 0 2.3-5.6M4 4v4h4",
};

export type IconName = keyof typeof PATHS;

export function Icon({
  name,
  size = 16,
  className = "",
}: {
  name: IconName;
  size?: number;
  className?: string;
}) {
  const filled = name === "play" || name === "stop";
  return (
    <svg
      className={`icon ${className}`}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth={filled ? 1.2 : 1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d={PATHS[name]} />
    </svg>
  );
}

// Brand mark: three stacked platters with a rising measurement line.
export function BrandMark({ size = 24 }: { size?: number }) {
  return (
    <svg className="mark" width={size} height={size} viewBox="0 0 24 24" aria-hidden>
      <rect x="3" y="4" width="18" height="4" rx="1.2" fill="currentColor" opacity="0.35" />
      <rect x="3" y="10" width="18" height="4" rx="1.2" fill="currentColor" opacity="0.6" />
      <rect x="3" y="16" width="18" height="4" rx="1.2" fill="currentColor" />
    </svg>
  );
}
