import { Activity, ListChecks, Monitor as MonitorIcon } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";

const navItems = [
  { to: "/", label: "Monitor", icon: MonitorIcon },
  { to: "/progress", label: "Progress", icon: ListChecks },
  { to: "/profiling", label: "Profiling", icon: Activity },
];

export default function Layout() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <nav className="monitor-top-nav">
        <div className="monitor-brand">Akita Monitor</div>
        <div className="monitor-nav-tabs" role="tablist" aria-label="Monitor views">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === "/"}
                role="tab"
                className={({ isActive }) =>
                  `monitor-nav-link ${isActive ? "monitor-nav-link-active" : ""}`
                }
              >
                <Icon className="h-4 w-4" />
                {item.label}
              </NavLink>
            );
          })}
        </div>
      </nav>
      <main className="monitor-main">
        <Outlet />
      </main>
    </div>
  );
}
