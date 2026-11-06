import { NavLink, Route, Routes } from "react-router-dom";
import { useLive } from "./state/LiveProvider";
import { useTheme } from "./state/ThemeProvider";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { BrandMark, Icon, type IconName } from "./components/Icon";
import { ACTIVE_PHASES } from "./domain";
import { Dashboard } from "./pages/Dashboard";
import { Profiles } from "./pages/Profiles";
import { ProfileEditor } from "./pages/ProfileEditor";
import { LiveView } from "./pages/LiveView";
import { Results } from "./pages/Results";
import { RunDetailPage } from "./pages/RunDetailPage";
import { BatchDetailPage } from "./pages/BatchDetailPage";
import { ComparePage } from "./pages/ComparePage";

const NAV: { to: string; label: string; icon: IconName; end?: boolean; live?: boolean }[] = [
  { to: "/", label: "Übersicht", icon: "dashboard", end: true },
  { to: "/live", label: "Live", icon: "live", live: true },
  { to: "/profiles", label: "Profile", icon: "profiles" },
  { to: "/results", label: "Ergebnisse", icon: "results" },
];

export function App() {
  const { connected, agents, testStates } = useLive();
  const { theme, toggle } = useTheme();
  const running = agents.filter((a) => ACTIVE_PHASES.has(testStates[a.agentId]?.phase ?? "")).length;

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <BrandMark />
          <div>
            storagebench
            <small>Storage-Benchmarks</small>
          </div>
        </div>
        {NAV.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.end}
            className={({ isActive }) => `nav-link ${isActive ? "active" : ""}`}
          >
            <Icon name={n.icon} />
            {n.label}
            {n.live && running > 0 && (
              <span className="count" title={`${running} Host(s) messen gerade`}>
                {running}
              </span>
            )}
          </NavLink>
        ))}
        <div className="sidebar-foot">
          <div className="conn" title={connected ? "Live-Verbindung zum Controller steht" : "Keine Live-Verbindung – Daten werden nicht aktualisiert"}>
            <span className={`live-dot ${connected ? "on" : "off"}`} />
            <span className="ellipsis">{connected ? "Verbunden" : "Keine Verbindung"}</span>
          </div>
          <button
            className="btn ghost icon sm"
            onClick={toggle}
            title={theme === "light" ? "Dunkles Design" : "Helles Design"}
            aria-label="Design umschalten"
          >
            <Icon name={theme === "light" ? "moon" : "sun"} />
          </button>
        </div>
      </aside>

      <div className="main">
        <main className="content">
          <ErrorBoundary>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/profiles" element={<Profiles />} />
              <Route path="/profiles/new" element={<ProfileEditor />} />
              <Route path="/profiles/:id" element={<ProfileEditor />} />
              <Route path="/live" element={<LiveView />} />
              <Route path="/results" element={<Results />} />
              <Route path="/compare" element={<ComparePage />} />
              <Route path="/runs/:id" element={<RunDetailPage />} />
              <Route path="/batches/:id" element={<BatchDetailPage />} />
            </Routes>
          </ErrorBoundary>
        </main>
      </div>
    </div>
  );
}
