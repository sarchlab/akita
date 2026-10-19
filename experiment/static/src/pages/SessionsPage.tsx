import { Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useScrollIntoView } from "../lib/useScrollIntoView";
import { useSearchParams } from "react-router-dom";
import SessionDetail from "../components/sessions/SessionDetail";
import SessionTable, { buildItems } from "../components/sessions/SessionTable";
import { PILL } from "../components/StatePill";
import Toggle from "../components/Toggle";
import { EmptyState } from "../components/Section";
import { Input } from "../components/ui/input";
import { isLive, sessionMtime } from "../lib/derive";
import { t } from "../lib/i18n";
import { matchesSearch } from "../lib/format";
import { stateLabel } from "../lib/notify";
import { setPrefs, usePrefs } from "../lib/prefs";
import { useStatusState } from "../lib/status";
import type { Session, Status } from "../lib/types";

const CHIP_ORDER = ["running", "stalled", "queued", "ok", "failed", "died", "stopped"];

function rulesText(st: Status): string {
  const r = st.rules || {};
  return t("sess.rules", { sim: r.sim_stall_minutes, work: r.work_stall_minutes, cpu: r.idle_cpu, stall: r.stall_minutes });
}

/** Sessions that pass every filter except state and archived. */
function baseRows(st: Status, p: ReturnType<typeof usePrefs>): Session[] {
  const now = st.updated ?? Date.now() / 1000;
  return st.sessions.filter((s) =>
    (!p.sessHost || s.host === p.sessHost)
    && !(p.recent && !isLive(s) && now - sessionMtime(s) > 86400)
    && matchesSearch(p.sessSearch, s.name, s.project, s.host, s.cmd, s.group));
}

export default function SessionsPage() {
  const { status } = useStatusState();
  const prefs = usePrefs();
  const [params, setParams] = useSearchParams();
  const [openGroups, setOpenGroups] = useState<Set<string>>(new Set());
  const selected = params.get("id");
  const detailRef = useScrollIntoView<HTMLElement>(selected, 1024);

  useEffect(() => {
    const host = params.get("host");
    if (host != null) {
      setPrefs({ sessHost: host });
      const next = new URLSearchParams(params);
      next.delete("host");
      setParams(next, { replace: true });
    }
  }, [params, setParams]);

  const view = useMemo(() => {
    if (!status) return null;
    const base = baseRows(status, prefs);
    const visible = base.filter((s) => prefs.showArchived || !s.archived);
    const counts: Record<string, number> = {};
    for (const s of visible) counts[s.state] = (counts[s.state] ?? 0) + 1;
    const rows = prefs.sessState ? visible.filter((s) => s.state === prefs.sessState) : visible;
    return { ...buildItems(rows, prefs.grouped), counts, rows: rows.length, archived: base.filter((s) => s.archived).length };
  }, [status, prefs]);

  // Selecting a member of a folded group opens the group.
  const sel = status?.sessions.find((s) => s.id === selected) ?? null;
  useEffect(() => {
    if (sel?.group && prefs.grouped) setOpenGroups((g) => (g.has(sel.group!) ? g : new Set(g).add(sel.group!)));
  }, [sel?.group, prefs.grouped]);

  if (!status || !view) return <div className="exp-page h-full"><EmptyState title={t("common.connecting")} hint={t("common.readingStatus")} /></div>;
  const now = status.updated ?? Date.now() / 1000;
  const select = (id: string) => {
    const next = new URLSearchParams(params);
    if (id === selected) next.delete("id"); else next.set("id", id);
    setParams(next, { replace: true });
  };

  return (
    <div className="exp-page flex h-full flex-col overflow-hidden">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b bg-card px-4 py-2">
        <div className="flex min-w-48 flex-1 items-center gap-2 sm:max-w-sm">
          <Search className="h-4 w-4 text-muted-foreground" />
          <Input value={prefs.sessSearch} placeholder={t("sess.search")} onChange={(e) => setPrefs({ sessSearch: e.target.value })} />
        </div>
        <select className="exp-select" value={prefs.sessHost} onChange={(e) => setPrefs({ sessHost: e.target.value })} aria-label={t("common.host")}>
          <option value="">{t("common.allHosts")}</option>
          {status.hosts.map((h) => <option key={h.name} value={h.name}>{h.name}</option>)}
        </select>
        <div className="flex flex-wrap gap-1.5">
          {CHIP_ORDER.map((s) => (
            <button key={s} type="button" title={t("sess.chipHint")}
              onClick={() => setPrefs({ sessState: prefs.sessState === s ? "" : s })}
              className={`rounded-full px-2 py-px text-xs font-semibold ${PILL[s]} ${view.counts[s] ? "" : "opacity-35"} ${prefs.sessState === s ? "ring-2 ring-foreground ring-offset-1 ring-offset-card" : ""}`}>
              {stateLabel(s)} {view.counts[s] ?? 0}
            </button>
          ))}
        </div>
        <Toggle checked={prefs.recent} onChange={(v) => setPrefs({ recent: v })}>{t("sess.recent")}</Toggle>
        <Toggle checked={prefs.grouped} onChange={(v) => setPrefs({ grouped: v })} title={t("sess.groupedHint")}>{t("sess.grouped")}</Toggle>
        <Toggle checked={prefs.showArchived} onChange={(v) => setPrefs({ showArchived: v })} title={t("sess.archivedHint")}>
          {t("common.showArchived", { n: view.archived })}
        </Toggle>
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">{t("sess.rulesTitle")}</summary>
          <p className="mt-1 max-w-3xl">{rulesText(status)}</p>
        </details>
      </div>
      <div className="min-h-0 flex-1 overflow-auto lg:grid lg:grid-cols-[minmax(0,1fr)_24rem] lg:overflow-hidden xl:grid-cols-[minmax(0,1fr)_30rem]">
        <div className="min-h-0 bg-card lg:overflow-auto">
          <div className="border-b px-4 py-2 text-xs text-muted-foreground">
            {t("sess.count", { n: view.rows })}{view.groups ? t("sess.groups", { n: view.groups }) : ""} · {t("sess.clickHint")}
          </div>
          {view.items.length ? (
            <div className="overflow-x-auto">
              <SessionTable items={view.items} now={now} selected={selected} openGroups={openGroups} onSelect={select}
                onToggleGroup={(g) => setOpenGroups((o) => { const n = new Set(o); if (n.has(g)) n.delete(g); else n.add(g); return n; })} />
            </div>
          ) : <EmptyState title={t("sess.none")} hint={prefs.recent ? t("sess.noneRecent") : t("sess.noneHint")} />}
        </div>
        <aside ref={detailRef} className="min-h-0 border-t bg-card lg:overflow-auto lg:border-l lg:border-t-0">
          {sel ? <SessionDetail s={sel} runs={status.runs} now={now} />
            : <EmptyState title={t("sess.noSel")} hint={t("sess.noSelHint")} />}
        </aside>
      </div>
    </div>
  );
}
