import { t } from "../lib/i18n";
import { SlidersHorizontal } from "lucide-react";
import { useMemo, useState } from "react";
import { useScrollIntoView } from "../lib/useScrollIntoView";
import { useNavigate, useSearchParams } from "react-router-dom";
import Facets from "../components/results/Facets";
import ResultCompare from "../components/results/ResultCompare";
import ResultList, { ListHeader, type ListActions } from "../components/results/ResultList";
import { EmptyState, SectionHeader } from "../components/Section";
import RunDetail from "../components/results/RunDetail";
import { matchesSearch } from "../lib/format";
import { buildFacets, filterRuns, groupRuns, namingOf, valueLabel } from "../lib/naming";
import { foldAll, MAX_PLOT, plotMany, plotOnly, setPrefs, toggleCmp, togglePlot, usePrefs } from "../lib/prefs";
import { useStatusState } from "../lib/status";
import { runKey, type Run, type Status } from "../lib/types";

const RESULT_LIMIT = 50;

function searchable(r: Run, st: Status): unknown[] {
  const naming = namingOf(st.projects, r.project);
  const f = r.fields ?? {};
  return [r.name, r.project, r.host, ...Object.entries(f).map(([k, v]) => valueLabel(naming, k, v))];
}

export default function ResultsPage() {
  const { status } = useStatusState();
  const prefs = usePrefs();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [showAll, setShowAll] = useState(false);
  const selected = params.get("run");
  const detailRef = useScrollIntoView<HTMLElement>(selected, 1280);

  const view = useMemo(() => {
    if (!status) return null;
    const live = status.runs.filter((r) => prefs.showArchived || !r.archived);
    const searched = live.filter((r) => matchesSearch(prefs.resSearch, ...searchable(r, status)));
    const facets = buildFacets(searched, status.projects, prefs.facets);
    const runs = filterRuns(searched, prefs.facets, status.projects);
    const limited = !showAll && !prefs.resSearch.trim();
    const grouped = groupRuns(runs, status.projects, limited ? RESULT_LIMIT : 0);
    const archived = filterRuns(status.runs.filter((r) => r.archived && matchesSearch(prefs.resSearch, ...searchable(r, status))), prefs.facets, status.projects).length;
    return { facets, runs, grouped, archived };
  }, [status, prefs.showArchived, prefs.resSearch, prefs.facets, showAll]);

  if (!status || !view) return <div className="exp-page h-full"><EmptyState title={t("common.connecting")} hint={t("common.readingStatus")} /></div>;
  const sel = status.runs.find((r) => runKey(r) === selected) ?? null;

  const full = () => window.alert(t("charts.full", { max: MAX_PLOT }));
  const actions: ListActions = {
    selected, cmp: prefs.cmp, plotSel: prefs.plot.sel,
    onSelect: (k) => {
      const next = new URLSearchParams(params);
      if (k === selected) next.delete("run"); else next.set("run", k);
      setParams(next, { replace: true });
    },
    onCmp: (k) => { if (!toggleCmp(k)) window.alert(t("res.cmpFull")); },
    onPlot: (k) => { plotOnly(k); navigate("/charts"); },
    onPlotAdd: (k) => { if (!togglePlot(k)) full(); },
    onPeers: (keys) => { plotMany(keys); navigate("/charts"); },
  };
  const { groups, hidden } = view.grouped;

  const sidebar = (
    <Facets facets={view.facets} sel={prefs.facets} onSel={(facets) => setPrefs({ facets })}
      search={prefs.resSearch} onSearch={(v) => setPrefs({ resSearch: v })}
      showArchived={prefs.showArchived} onArchived={(v) => setPrefs({ showArchived: v })}
      archivedCount={view.archived} total={view.runs.length} />
  );

  return (
    <div className="exp-page h-full overflow-auto md:flex md:overflow-hidden">
      <aside className="border-b bg-card md:w-72 md:shrink-0 md:overflow-auto md:border-b-0 md:border-r">
        <details className="md:hidden" open={false}>
          <summary className="flex cursor-pointer items-center gap-2 px-3 py-2 text-sm font-medium">
            <SlidersHorizontal className="h-4 w-4" />{t("res.filters")}
            <span className="ml-auto text-xs text-muted-foreground">{view.runs.length}</span>
          </summary>
          {sidebar}
        </details>
        <div className="max-md:hidden">{sidebar}</div>
      </aside>
      <div className="min-w-0 md:flex-1 md:overflow-auto xl:grid xl:grid-cols-[minmax(0,1fr)_24rem] xl:overflow-hidden">
        <div className="min-h-0 xl:overflow-auto">
          {prefs.cmp.length ? (
            <div className="px-3 pt-3"><ResultCompare cmp={prefs.cmp} onRemove={(k) => toggleCmp(k)} /></div>
          ) : null}
          <div className="@container flex flex-col gap-3 p-3">
            <SectionHeader title={t("res.title")}
              subtitle={t("res.sub", { n: view.runs.length, by: groupByText(status, view.runs) }) + (hidden ? t("res.subLimit", { n: RESULT_LIMIT }) : "")}
              right={<>
                <button type="button" className="rounded border bg-card px-2 py-1 text-xs hover:bg-muted" onClick={() => foldAll(false)}>{t("res.foldAll")}</button>
                <button type="button" className="rounded border bg-card px-2 py-1 text-xs hover:bg-muted" onClick={() => foldAll(true)}>{t("res.openAll")}</button>
              </>} />
            {groups.length ? <>
              <ListHeader />
              <ResultList groups={groups} projects={status.projects} allRuns={status.runs} showArchived={prefs.showArchived} a={actions} />
            </> : <EmptyState title={t("res.none")} hint={t("res.noneHint")} />}
          </div>
          {hidden ? (
            <div className="flex flex-wrap items-center gap-3 px-4 pb-4 text-sm">
              <span className="text-muted-foreground">{t("res.hidden", { n: hidden })}</span>
              <button type="button" className="rounded border bg-card px-2 py-1 text-xs hover:bg-muted" onClick={() => setShowAll(true)}>{t("res.showAll")}</button>
            </div>
          ) : showAll && !prefs.resSearch.trim() && view.grouped.total > RESULT_LIMIT ? (
            <div className="px-4 pb-4">
              <button type="button" className="rounded border bg-card px-2 py-1 text-xs hover:bg-muted" onClick={() => setShowAll(false)}>{t("res.showRecent", { n: RESULT_LIMIT })}</button>
            </div>
          ) : null}
        </div>
        <aside ref={detailRef} className="min-h-0 border-t bg-card xl:overflow-auto xl:border-l xl:border-t-0">
          {sel ? (
            <RunDetail run={sel} projects={status.projects}
              cmpIndex={prefs.cmp.indexOf(runKey(sel))} plotIndex={prefs.plot.sel.indexOf(runKey(sel))}
              onCmp={() => actions.onCmp(runKey(sel))} onPlot={() => actions.onPlot(runKey(sel))} onPlotAdd={() => actions.onPlotAdd(runKey(sel))} />
          ) : <EmptyState title={t("res.noSel")} hint={t("res.noSelHint")} />}
        </aside>
      </div>
    </div>
  );
}

function groupByText(st: Status, runs: Run[]): string {
  const p = runs.find((r) => namingOf(st.projects, r.project)?.group_by?.length)?.project;
  const n = p ? namingOf(st.projects, p) : null;
  if (!n?.group_by?.length) return t("res.namingRule");
  return n.group_by.map((f) => n.fields.find((x) => x.name === f)?.label || f).join(" + ");
}
