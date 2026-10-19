import { BarChart3, Check, ChevronDown, ChevronRight, GitCompare, Plus } from "lucide-react";
import type { ReactNode } from "react";
import { fmtTimeShort, utcTitle } from "../../lib/format";
import { campaignRun } from "../../lib/derive";
import { groupPeers, type Projects, type RunGroup } from "../../lib/naming";
import { isGroupOpen, MAX_PLOT, toggleGroup, usePrefs } from "../../lib/prefs";
import { keyResults } from "../../lib/summary";
import { t } from "../../lib/i18n";
import { unparsedLabel } from "../../lib/naming";
import { runKey, type Run } from "../../lib/types";
import RunName from "../RunName";
import { ArchivedTag } from "../StatePill";
import RunStatus, { outcomeOf } from "./RunStatus";

export interface ListActions {
  selected: string | null;
  cmp: string[];
  plotSel: string[];
  onSelect: (key: string) => void;
  onCmp: (key: string) => void;
  onPlot: (key: string) => void;
  onPlotAdd: (key: string) => void;
  onPeers: (keys: string[]) => void;
}

const LETTERS = "ABCD";

/**
 * Fixed tracks shared by every row of every group (a container query, so they
 * apply when the list itself is wide enough): checkbox | name | key result |
 * host | status | time | actions. Narrower lists stack the cells.
 */
export const ROW_GRID = "grid grid-cols-[1.25rem_minmax(0,1fr)] gap-x-3 gap-y-1 @2xl:grid-cols-[1.25rem_minmax(9rem,1fr)_6.5rem_3rem_8.5rem_5.75rem_4rem] @2xl:items-center";

function IconButton({ onClick, title, active, children }: { onClick: () => void; title: string; active?: boolean; children: ReactNode }) {
  return (
    <button type="button" title={title} aria-label={title}
      onClick={(e) => { e.stopPropagation(); onClick(); }}
      className={`inline-flex h-7 min-w-7 items-center justify-center gap-0.5 rounded border px-1 text-[11px] font-semibold ${active ? "border-primary bg-primary text-primary-foreground" : "bg-card text-primary hover:bg-primary/10"}`}>
      {children}
    </button>
  );
}

function RunRow({ run, projects, a, parsed }: { run: Run; projects: Projects; a: ListActions; parsed: boolean }) {
  const key = runKey(run);
  const at = a.plotSel.indexOf(key);
  const ci = a.cmp.indexOf(key);
  const kr = keyResults(run);
  return (
    <div onClick={() => a.onSelect(key)}
      className={`${ROW_GRID} cursor-pointer border-b px-3 py-2 text-sm last:border-b-0 ${a.selected === key ? "exp-row-selected" : "exp-row"} ${run.archived ? "opacity-60" : ""}`}>
      <input type="checkbox" className="mt-0.5 h-4 w-4 accent-primary @2xl:mt-0" title={t("rl.cmpHint")} aria-label={t("rl.cmp")} checked={ci >= 0}
        onClick={(e) => e.stopPropagation()} onChange={() => a.onCmp(key)} />
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
        {parsed ? <RunName run={run} projects={projects} /> : <span className="break-all font-mono text-xs">{run.name}</span>}
        {ci >= 0 ? <span className="text-[11px] font-semibold text-primary">{t("rl.cmpN", { l: LETTERS[ci] })}</span> : null}
        {run.kind === "sensitivity" ? <span className="rounded border px-1 text-[11px] text-muted-foreground">{t("common.sensitivity")}</span> : null}
        {run.archived ? <ArchivedTag /> : null}
        {run.deviation?.length ? <span className="rounded border border-amber-500 px-1 text-[11px] text-amber-700 dark:text-amber-300" title={t("rl.devHint")}>{t("rl.dev", { flags: run.deviation.join(" ") })}</span> : null}
      </div>
      <div className="col-start-2 flex flex-wrap items-center gap-x-3 gap-y-1 @2xl:contents">
      <div className={`truncate font-mono text-[11px] text-muted-foreground`} title={kr.join("\n")}>{kr[0] ?? ""}</div>
      <div className={`truncate text-xs text-muted-foreground`}>{run.host}</div>
      <div className="min-w-28"><RunStatus run={run} /></div>
      <div className={`whitespace-nowrap font-mono text-[11px] tabular-nums text-muted-foreground`} title={utcTitle(run.mtime)}>{fmtTimeShort(run.mtime)}</div>
      <div className={`flex gap-1 @2xl:justify-end`}>
        {campaignRun(run) ? <>
          <IconButton onClick={() => a.onPlot(key)} title={t("rl.plotHint")}><BarChart3 className="h-3.5 w-3.5" /></IconButton>
          <IconButton onClick={() => a.onPlotAdd(key)} active={at >= 0}
            title={at >= 0 ? t("rl.inChart", { l: LETTERS[at] }) : t("rl.addChart", { max: MAX_PLOT })}>
            {at >= 0 ? <><Check className="h-3 w-3" />{LETTERS[at]}</> : <Plus className="h-3.5 w-3.5" />}
          </IconButton>
        </> : null}
      </div>
      </div>
    </div>
  );
}

function GroupSummary({ runs }: { runs: Run[] }) {
  const c = { ok: 0, failed: 0, other: 0 };
  for (const r of runs) c[outcomeOf(r)]++;
  return (
    <span className="flex items-center gap-2 text-xs">
      <span className="rounded-full bg-muted px-1.5 font-mono font-semibold tabular-nums text-foreground" title={t("rl.dirs")}>{runs.length}</span>
      {c.ok ? <span className="text-emerald-700 dark:text-emerald-400">{t("rl.ok", { n: c.ok })}</span> : null}
      {c.failed ? <span className="text-red-600 dark:text-red-400">{t("progress.failed", { n: c.failed })}</span> : null}
      {c.other ? <span className="text-muted-foreground">{t("rl.other", { n: c.other })}</span> : null}
    </span>
  );
}

/** Column titles, shown when the rows use the fixed tracks. */
export function ListHeader() {
  return (
    <div className={`${ROW_GRID} hidden px-3 text-[11px] font-medium text-muted-foreground @2xl:grid`}>
      <span />
      <span>{t("rl.colName")}</span><span>{t("rc.key")}</span><span>{t("common.host")}</span><span>{t("sd.state")}</span><span>{t("rl.colUpdated")}</span><span className="text-right">{t("rl.colActions")}</span>
    </div>
  );
}

export default function ResultList({ groups, projects, allRuns, showArchived, a }: {
  groups: RunGroup[]; projects: Projects; allRuns: Run[]; showArchived: boolean; a: ListActions;
}) {
  const prefs = usePrefs();
  return (
    <div className="flex flex-col gap-3">
      {groups.map((g) => {
        const gk = g.key || "\u0000unparsed";
        const open = isGroupOpen(prefs, gk);
        const peers = g.parsed ? groupPeers(allRuns, projects, g.key, showArchived) : [];
        const Chevron = open ? ChevronDown : ChevronRight;
        return (
          <section key={gk} className="overflow-hidden rounded-lg border bg-card">
            <div className={`flex flex-wrap items-center gap-x-3 gap-y-1 bg-muted/40 px-3 py-2 ${open ? "border-b" : ""}`}>
              <button type="button" className="flex min-w-0 items-center gap-1.5 text-left" aria-expanded={open} onClick={() => toggleGroup(gk)}>
                <Chevron className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate text-base font-semibold">{g.parsed ? g.label : unparsedLabel()}</span>
              </button>
              <span className="text-xs text-muted-foreground">{g.project}</span>
              <GroupSummary runs={g.runs} />
              {!g.parsed ? <span className="text-xs text-muted-foreground">{t("rl.unparsed")}</span> : null}
              {peers.length >= 2 ? (
                <button type="button" className="ml-auto inline-flex items-center gap-1 rounded border bg-card px-2 py-0.5 text-xs font-medium text-primary hover:bg-primary/10"
                  title={t("rl.peersHint", { max: MAX_PLOT })} onClick={() => a.onPeers(peers.map(runKey))}>
                  <GitCompare className="h-3.5 w-3.5" />{t("rl.peers")}
                </button>
              ) : null}
            </div>
            {open ? g.runs.map((r) => <RunRow key={r.id} run={r} projects={projects} a={a} parsed={g.parsed} />) : null}
          </section>
        );
      })}
    </div>
  );
}
