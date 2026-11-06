import { NavLink, Route, Routes } from "react-router-dom";
import { useLive } from "./state/LiveProvider";
import { Dashboard } from "./pages/Dashboard";
import { Wizard } from "./pages/Wizard";
import { LiveView } from "./pages/LiveView";
import { Results } from "./pages/Results";
import { RunDetailPage } from "./pages/RunDetailPage";
import { ErrorBoundary } from "./components/ErrorBoundary";

export function App() {
  const { connected } = useLive();
  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">storagebench</div>
        <nav className="nav">
          <NavLink to="/" end>
            Dashboard
          </NavLink>
          <NavLink to="/wizard">Neuer Test</NavLink>
          <NavLink to="/live">Live</NavLink>
          <NavLink to="/results">Ergebnisse</NavLink>
        </nav>
        <div className={`conn ${connected ? "ok" : "down"}`}>
          {connected ? "verbunden" : "getrennt"}
        </div>
      </header>
      <main className="content">
        <ErrorBoundary>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/wizard" element={<Wizard />} />
            <Route path="/live" element={<LiveView />} />
            <Route path="/results" element={<Results />} />
            <Route path="/runs/:id" element={<RunDetailPage />} />
          </Routes>
        </ErrorBoundary>
      </main>
    </div>
  );
}
