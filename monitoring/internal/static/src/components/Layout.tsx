import { Activity, Bug, Gauge, ListChecks, Monitor as MonitorIcon } from "lucide-react";
import { NavLink, Outlet } from "react-router-dom";
import { PropertyMonitoringCollector } from "../hooks/usePropertyMonitoringSamples";
import { ResourceUsageCollector } from "../hooks/useResourceUsageHistory";

const navItems = [
  { to: "/execution", label: "Execution", icon: ListChecks },
  { to: "/monitor", label: "Monitor", icon: MonitorIcon },
  { to: "/analysis", label: "Analysis", icon: Gauge },
  { to: "/debug", label: "Debug", icon: Bug },
  { to: "/profiling", label: "Profiling", icon: Activity },
];

export default function Layout() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PropertyMonitoringCollector />
      <ResourceUsageCollector />
      <nav className="monitor-top-nav">
        <div className="monitor-brand">AkitaRTM</div>
        <div className="monitor-nav-tabs" role="tablist" aria-label="Monitor views">
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end
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
