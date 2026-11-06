import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Icon } from "./Icon";
import { fmtClock, fmtDateTime, fmtRelative } from "../format";

interface Props {
  title: ReactNode;
  crumbs?: { to: string; label: string }[];
  meta?: ReactNode;
  actions?: ReactNode;
}

// PageHeader: one place for "where am I" (breadcrumb + title), the object's
// state (meta) and the page's primary actions.
export function PageHeader({ title, crumbs, meta, actions }: Props) {
  return (
    <header className="page-head">
      <div style={{ minWidth: 0 }}>
        {crumbs && crumbs.length > 0 && (
          <nav className="crumbs" aria-label="Pfad">
            {crumbs.map((c) => (
              <span key={c.to} className="row" style={{ gap: 6 }}>
                <Link to={c.to}>{c.label}</Link>
                <Icon name="right" size={12} />
              </span>
            ))}
          </nav>
        )}
        <h1 className="page-title">{title}</h1>
        {meta && <div className="page-meta">{meta}</div>}
      </div>
      {actions && <div className="page-actions">{actions}</div>}
    </header>
  );
}

export function Empty({
  title,
  children,
  action,
  compact,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
  compact?: boolean;
}) {
  return (
    <div className={`empty ${compact ? "compact" : ""}`}>
      <div className="title">{title}</div>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}

// TimeCell: relative time for scanning, absolute time for precision.
export function TimeCell({ iso }: { iso: string | undefined }) {
  return (
    <span className="when" title={fmtDateTime(iso)}>
      <span className="rel">{fmtRelative(iso)}</span>
      <span className="abs">{fmtClock(iso)}</span>
    </span>
  );
}
