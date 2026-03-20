import { useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router";
import { useMode } from "../hooks/useMode";
import EngineControlPanel from "./live/EngineControlPanel";
import TracingControls from "./live/TracingControls";
import ChatPanel from "./chat/ChatPanel";

/**
 * Layout — root layout component with navbar.
 *
 * In live mode the navbar shows additional links (Monitor, Execution)
 * and engine/tracing controls. The /live route renders a three-pane
 * PanelLayout (via LivePage) that fills the viewport below the navbar,
 * so we suppress the default <main> padding for that route.
 */
export default function Layout() {
  const { mode, loading } = useMode();
  const location = useLocation();
  const [chatOpen, setChatOpen] = useState(false);

  /* The /live route uses PanelLayout which manages its own spacing. */
  const isLivePanel = location.pathname === "/live";

  return (
    <div className="d-flex flex-column min-vh-100">
      <nav className="navbar navbar-expand navbar-dark bg-dark px-3">
        <span className="navbar-brand">
          Daisen
          {!loading && mode === "live" && (
            <span className="badge bg-success ms-2 align-middle" style={{ fontSize: 10 }}>
              live
            </span>
          )}
        </span>

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
          <li className="nav-item">
            <NavLink className="nav-link" to="/metrics">
              Metrics
            </NavLink>
          </li>
          {mode === "live" && (
            <li className="nav-item">
              <NavLink className="nav-link" to="/live">
                Monitor
              </NavLink>
            </li>
          )}
          {mode === "live" && (
            <li className="nav-item">
              <NavLink className="nav-link" to="/live/execution">
                Execution
              </NavLink>
            </li>
          )}
        </ul>

        {/* Live-mode controls */}
        {mode === "live" && (
          <div className="d-flex align-items-center gap-3 me-3">
            <EngineControlPanel />
            <TracingControls />
          </div>
        )}

        <button
          className="btn btn-sm btn-outline-light"
          onClick={() => setChatOpen((prev) => !prev)}
          type="button"
        >
          AI Chat
        </button>
      </nav>

      {isLivePanel ? (
        /* PanelLayout fills the viewport — no padding wrapper */
        <Outlet />
      ) : (
        <main className="flex-grow-1 p-3">
          <Outlet />
        </main>
      )}

      <ChatPanel isOpen={chatOpen} onClose={() => setChatOpen(false)} />
    </div>
  );
}
