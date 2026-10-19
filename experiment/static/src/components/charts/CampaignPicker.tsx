import { Check, Plus, Search } from "lucide-react";
import { useState } from "react";
import { campaignRun } from "../../lib/derive";
import { t } from "../../lib/i18n";
import { MAX_PLOT } from "../../lib/prefs";
import { matchesSearch } from "../../lib/format";
import { groupLabel, namingOf } from "../../lib/naming";
import { runKey, type Status } from "../../lib/types";
import { ProgressTrack } from "../ProgressBar";
import RunName from "../RunName";
import { ArchivedTag } from "../StatePill";
import { Input } from "../ui/input";
import { EmptyState } from "../Section";

const L = "ABCD";

/** Sidebar of campaigns to add to the chart, and plotting by directory name. */
export default function CampaignPicker({ status, sel, showArchived, onToggle }: {
  status: Status; sel: string[]; showArchived: boolean; onToggle: (key: string) => void;
}) {
  const [q, setQ] = useState("");
  const [host, setHost] = useState(status.hosts[0]?.name ?? "");
  const [name, setName] = useState("");
  const camps = status.runs
    .filter((r) => campaignRun(r) && (showArchived || !r.archived || sel.includes(runKey(r))))
    .filter((r) => matchesSearch(q, r.name, r.host, r.project, r.fields ? groupLabel(namingOf(status.projects, r.project), r.fields) : ""))
    .sort((a, b) => (b.mtime ?? 0) - (a.mtime ?? 0));

  const byName = () => {
    let n = name.trim().replace(/\/+$/, "");
    if (!host || !n) return;
    let project: string | undefined;
    if (n.includes("/")) [project, n] = n.split("/").slice(-2);
    if (!project) {
      const counts: Record<string, number> = {};
      for (const r of status.runs) if (r.host === host) counts[r.project] = (counts[r.project] || 0) + 1;
      project = Object.keys(counts).sort((a, b) => counts[b] - counts[a])[0];
    }
    if (!project) { window.alert(t("picker.needProject")); return; }
    onToggle(`${host}|${project}|${n}`);
    setName("");
  };

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <div className="mb-2 flex items-center gap-2">
          <Search className="h-4 w-4 text-muted-foreground" />
          <Input value={q} placeholder={t("picker.filter")} onChange={(e) => setQ(e.target.value)} />
        </div>
        <div className="text-xs text-muted-foreground">{t("picker.count", { n: camps.length, max: MAX_PLOT })}</div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto max-md:max-h-72">
        {camps.map((r) => {
          const k = runKey(r), at = sel.indexOf(k);
          return (
            <button key={r.id} type="button" onClick={() => onToggle(k)}
              className={`block w-full border-b px-3 py-2 text-left text-sm ${at >= 0 ? "bg-primary/10" : "hover:bg-muted/70"} ${r.archived ? "opacity-60" : ""}`}>
              <div className="flex items-start gap-2">
                <span className={`mt-0.5 inline-flex h-4 w-4 shrink-0 items-center justify-center rounded text-[10px] font-bold ${at >= 0 ? "bg-primary text-primary-foreground" : "border text-muted-foreground"}`}>
                  {at >= 0 ? L[at] : <Plus className="h-3 w-3" />}
                </span>
                <span className="min-w-0 flex-1">
                  <RunName run={r} projects={status.projects} withGroup />
                  {r.archived ? <ArchivedTag /> : null}
                  <span className="mt-0.5 flex items-center gap-2 text-[11px] text-muted-foreground">
                    {r.host}{r.kind === "sensitivity" ? ` · ${t("common.sensitivity")}` : ""}
                    {r.progress ? <><span className="font-mono">{r.progress.finished}/{r.progress.total ?? "?"}</span>
                      <ProgressTrack bar={r.progress} className="h-1.5 w-16" /></> : null}
                  </span>
                </span>
                {at >= 0 ? <Check className="h-4 w-4 shrink-0 text-primary" /> : null}
              </div>
            </button>
          );
        })}
        {!camps.length ? <EmptyState className="p-6" title={t("picker.none")} hint={q ? t("picker.noneSearch") : t("picker.noneRecent")} /> : null}
      </div>
      <div className="border-t p-3">
        <div className="mb-1.5 text-xs text-muted-foreground">{t("picker.byName")}</div>
        <div className="flex gap-1.5">
          <select className="exp-select" value={host} onChange={(e) => setHost(e.target.value)} aria-label={t("common.host")}>
            {status.hosts.map((h) => <option key={h.name}>{h.name}</option>)}
          </select>
          <Input className="h-8" value={name} placeholder={t("picker.namePlaceholder")} onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") byName(); }} />
        </div>
        <button type="button" className="mt-1.5 w-full rounded border bg-card px-2 py-1 text-xs hover:bg-muted" onClick={byName}>{t("picker.add")}</button>
      </div>
    </div>
  );
}
