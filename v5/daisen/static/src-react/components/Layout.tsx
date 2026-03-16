import { NavLink, Outlet } from "react-router";
import { useMode } from "../hooks/useMode";
import EngineControlPanel from "./live/EngineControlPanel";
import TracingControls from "./live/TracingControls";

export default function Layout() {
  const { mode, loading } = useMode();

  return (
    <div className="d-flex flex-column min-vh-100">
      <nav className="navbar navbar-expand navbar-dark bg-dark px-3">
        <span className="navbar-brand">Daisen</span>

        <ul className="navbar-nav me-auto">
          <li className="nav-item">
            <NavLink className="nav-link" to="/" end>
              Dashboard
            </NavLink>
          </li>
          <li className="nav-item">
            <NavLink className="nav-link" to="/task">
              Tasks
            </NavLink>
          </li>
          <li className="nav-item">
            <NavLink className="nav-link" to="/component">
              Components
            </NavLink>
          </li>
        </ul>

        {/* Live-mode controls */}
        {mode === "live" && (
          <div className="d-flex align-items-center gap-3 me-3">
            <EngineControlPanel />
            <TracingControls />
          </div>
        )}

        <span className="navbar-text">
          {loading ? "loading…" : `Mode: ${mode ?? "unknown"}`}
        </span>
      </nav>

      <main className="flex-grow-1 p-3">
        <Outlet />
      </main>
    </div>
  );
}
