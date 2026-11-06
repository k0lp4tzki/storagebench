import { NavLink, Route, Routes, useLocation } from "react-router-dom";
import { useLive } from "./state/LiveProvider";
import { useTheme } from "./state/ThemeProvider";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { Dashboard } from "./pages/Dashboard";
import { Profiles } from "./pages/Profiles";
import { ProfileEditor } from "./pages/ProfileEditor";
import { LiveView } from "./pages/LiveView";
import { Results } from "./pages/Results";
import { RunDetailPage } from "./pages/RunDetailPage";
import { BatchDetailPage } from "./pages/BatchDetailPage";

const NAV = [
  { to: "/", label: "Dashboard", icon: "◧", end: true },
  { to: "/profiles", label: "Profile", icon: "☰" },
  { to: "/live", label: "Live", icon: "◉" },
  { to: "/results", label: "Ergebnisse", icon: "▤" },
];

const TITLES: Record<string, string> = {
  "/": "Dashboard",
  "/profiles": "Profile",
  "/live": "Live-Ansicht",
  "/results": "Ergebnisse",
};

export function App() {
  const { connected } = useLive();
  const { theme, toggle } = useTheme();
  const loc = useLocation();

  let title = TITLES[loc.pathname] ?? "";
  if (loc.pathname.startsWith("/profiles/")) title = "Profil bearbeiten";
  else if (loc.pathname.startsWith("/runs/")) title = "Testlauf";
  else if (loc.pathname.startsWith("/batches/")) title = "Profillauf";

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">
          <span className="logo">S</span>
          storagebench
        </div>
        {NAV.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.end}
            className={({ isActive }) => `nav-link ${isActive ? "active" : ""}`}
          >
            <span className="ico">{n.icon}</span>
            {n.label}
          </NavLink>
        ))}
        <div className="spacer" />
        <div className="nav-link" style={{ cursor: "default" }}>
          <span className="ico">{connected ? "🟢" : "🔴"}</span>
          {connected ? "verbunden" : "getrennt"}
        </div>
      </aside>

      <div className="main">
        <header className="topbar">
          <div className="title">{title}</div>
          <div className="right">
            <button
              className="btn ghost icon"
              onClick={toggle}
              title={theme === "light" ? "Dunkles Design" : "Helles Design"}
              aria-label="Theme umschalten"
            >
              {theme === "light" ? "🌙" : "☀️"}
            </button>
          </div>
        </header>

        <main className="content">
          <ErrorBoundary>
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/profiles" element={<Profiles />} />
              <Route path="/profiles/new" element={<ProfileEditor />} />
              <Route path="/profiles/:id" element={<ProfileEditor />} />
              <Route path="/live" element={<LiveView />} />
              <Route path="/results" element={<Results />} />
              <Route path="/runs/:id" element={<RunDetailPage />} />
              <Route path="/batches/:id" element={<BatchDetailPage />} />
            </Routes>
          </ErrorBoundary>
        </main>
      </div>
    </div>
  );
}
