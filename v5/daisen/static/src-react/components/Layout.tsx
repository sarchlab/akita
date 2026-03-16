import { NavLink, Outlet } from "react-router";
import { useMode } from "../hooks/useMode";

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
