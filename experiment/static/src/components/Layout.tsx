import { useEffect, useState } from "react";
import { BarChart3, Bell, BellOff, FolderOpen, Languages, LayoutDashboard, ListOrdered, Terminal } from "lucide-react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { fmtDur, fmtTime, utcTitle } from "../lib/format";
import { getLocale, setLocale, t, useLocale } from "../lib/i18n";
import { startPolling, useStatusState } from "../lib/status";
import type { Host } from "../lib/types";

const navItems = [
  { to: "/", label: "nav.overview", hint: "nav.overview.hint", icon: LayoutDashboard },
  { to: "/sessions", label: "nav.sessions", hint: "nav.sessions.hint", icon: Terminal },
  { to: "/results", label: "nav.results", hint: "nav.results.hint", icon: FolderOpen },
  { to: "/charts", label: "nav.charts", hint: "nav.charts.hint", icon: BarChart3 },
  { to: "/queue", label: "nav.queue", hint: "nav.queue.hint", icon: ListOrdered },
] as const;

function notifyText(): { text: string; on: boolean; disabled: boolean } {
  if (typeof window === "undefined" || !("Notification" in window)) return { text: t("notify.unsupported"), on: false, disabled: true };
  if (Notification.permission === "granted") return { text: t("notify.on"), on: true, disabled: true };
  if (Notification.permission === "denied") return { text: t("notify.denied"), on: false, disabled: true };
  return { text: t("notify.enable"), on: false, disabled: false };
}

/** Host type as a short word ("local", "ssh", "slurm"). */
export function hostType(h: Host): string {
  const k = `host.type.${h.type || "ssh"}`;
  return k === "host.type.local" || k === "host.type.ssh" || k === "host.type.slurm" ? t(k) : h.type || "";
}

function LangButton() {
  const loc = useLocale();
  return (
    <button type="button" onClick={() => setLocale(getLocale() === "zh" ? "en" : "zh")} title={t("lang.switch")}
      className="inline-flex items-center gap-1.5 rounded-md border border-slate-700 px-2 py-1 text-xs font-medium text-slate-200 hover:bg-slate-800">
      <Languages className="h-3.5 w-3.5" />
      <span>{loc === "zh" ? "EN" : t("lang.zhName")}</span>
    </button>
  );
}

function NotifyButton() {
  const [st, setSt] = useState(notifyText);
  const loc = useLocale();
  useEffect(() => { setSt(notifyText()); }, [loc]);
  return (
    <button
      type="button"
      disabled={st.disabled}
      onClick={() => { void Notification.requestPermission().then(() => setSt(notifyText())); }}
      className="inline-flex items-center gap-1.5 rounded-md border border-slate-700 px-2 py-1 text-xs font-medium text-slate-200 hover:bg-slate-800 disabled:cursor-default disabled:opacity-70 disabled:hover:bg-transparent"
      title={t("notify.hint")}
    >
      {st.on ? <Bell className="h-3.5 w-3.5" /> : <BellOff className="h-3.5 w-3.5" />}
      <span className="hidden md:inline">{st.text}</span>
    </button>
  );
}

function StatusInfo() {
  const { status, error, paused } = useStatusState();
  const [, tick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => tick((x) => x + 1), 5000);
    return () => window.clearInterval(id);
  }, []);
  const updated = status?.updated ?? null;
  const limit = 3 * (status?.interval || 10) + 30;
  const age = updated != null ? Date.now() / 1000 - updated : 0;
  const stale = updated != null && age > limit;
  return (
    <div className="ml-auto flex min-w-0 items-center gap-3 pl-3 text-xs text-slate-300">
      {paused ? <span className="hidden text-amber-300 sm:inline">{t("status.paused")}</span> : null}
      {error ? (
        <span className="truncate text-red-300" title={error}>{t("status.noServer")}</span>
      ) : (
        <span className="hidden whitespace-nowrap font-mono tabular-nums sm:inline" title={utcTitle(updated)}>
          {updated ? t("status.updated", { time: fmtTime(updated) }) : t("status.connecting")}
        </span>
      )}
      {!error && stale ? (
        <span className="whitespace-nowrap rounded-full bg-amber-400/90 px-2 py-px text-[11px] font-semibold text-slate-950"
          title={t("status.staleHint", { age: fmtDur(age), limit })}>
          {t("status.stale")}
        </span>
      ) : null}
      <span className="flex items-center gap-2">
        {(status?.hosts ?? []).map((h) => (
          <span key={h.name} className="inline-flex items-center gap-1"
            title={h.ok ? t("host.fetch", { name: h.name, type: hostType(h), s: h.fetch_s ?? "?" }) : t("host.error", { name: h.name, type: hostType(h), error: h.error })}>
            <span className={`h-2 w-2 rounded-full ${h.ok ? "bg-emerald-400" : "bg-red-500"}`} />
            <span className="hidden lg:inline">{h.name}</span>
            <span className="hidden text-[10px] text-slate-400 xl:inline">{hostType(h)}</span>
          </span>
        ))}
      </span>
      <LangButton />
      <NotifyButton />
    </div>
  );
}

export default function Layout() {
  const locale = useLocale();
  const location = useLocation();
  const navigate = useNavigate();
  useEffect(() => { startPolling(); }, []);
  // Old shareable links (/?plot=..., /?cmp=...) open the page that shows them.
  useEffect(() => {
    const q = new URLSearchParams(location.search);
    if (q.get("plot") && location.pathname !== "/charts") navigate(`/charts${location.search}`, { replace: true });
    else if (q.get("cmp") && location.pathname !== "/results") navigate(`/results${location.search}`, { replace: true });
  }, [location, navigate]);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <nav className="monitor-top-nav">
        <div className="monitor-brand max-sm:mr-2 max-sm:text-base">AkitaExp</div>
        <div className="monitor-nav-tabs" role="tablist" aria-label={t("nav.pages")}>
          {navItems.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end
                role="tab"
                title={t(item.hint)}
                className={({ isActive }) => `monitor-nav-link max-sm:px-2 ${isActive ? "monitor-nav-link-active" : ""}`}
              >
                <Icon className="h-4 w-4" />
                <span className="max-sm:hidden">{t(item.label)}</span>
              </NavLink>
            );
          })}
        </div>
        <StatusInfo />
      </nav>
      <main className="monitor-main" key={locale}>
        <Outlet />
      </main>
    </div>
  );
}
