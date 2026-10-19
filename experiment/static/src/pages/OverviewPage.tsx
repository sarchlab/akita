import { useNavigate } from "react-router-dom";
import { AlertTriangle, BarChart3 } from "lucide-react";
import Meter from "../components/Meter";
import ProgressBar from "../components/ProgressBar";
import RunName from "../components/RunName";
import StatePill, { Pill } from "../components/StatePill";
import WidgetCard from "../components/WidgetCard";
import { campaignDone, campaignRun, isActive, isLive, runFailed, sessionMtime } from "../lib/derive";
import { EmptyState } from "../components/Section";
import StatTile from "../components/StatTile";
import { fmtDur, fmtSim, fmtTime, utcTitle } from "../lib/format";
import { plotOnly } from "../lib/prefs";
import { useStatusState } from "../lib/status";
import type { Host, Session, Status } from "../lib/types";
import { t } from "../lib/i18n";
import { hostType } from "../components/Layout";

function HostCard({ h, status }: { h: Host; status: Status }) {
  const running = status.sessions.filter((s) => s.host === h.name && isActive(s)).length;
  if (!h.ok) {
    return (
      <WidgetCard title={<>{h.name} <span className="text-xs font-normal text-muted-foreground">{hostType(h)}</span></>} headerRight={<Pill kind="failed">{t("ov.unreachable")}</Pill>}>
        <div className="whitespace-pre-wrap break-all text-xs text-red-600 dark:text-red-400">{h.error}</div>
      </WidgetCard>
    );
  }
  const load = h.load?.[0] ?? 0, cpus = h.cpus || 1;
  const total = h.mem_total_gb ?? 0, avail = h.mem_avail_gb ?? 0;
  return (
    <WidgetCard title={<>{h.name} <span className="text-xs font-normal text-muted-foreground">{hostType(h)}</span></>} expandHref={`/sessions?host=${encodeURIComponent(h.name)}`}
      headerRight={<span className="text-xs text-muted-foreground">{t("ov.hostHead", { n: running, s: h.fetch_s ?? "?" })}</span>}>
      <div className="flex flex-col gap-2">
        <Meter label={t("ov.cpuLoad")} frac={load / cpus} text={t("ov.cores", { load: load.toFixed(1), cpus })} />
        <Meter label={t("ov.mem")} frac={total ? 1 - avail / total : 0} text={`${(total - avail).toFixed(0)} / ${total.toFixed(0)} GB`} />
        {(h.gpus ?? []).map((g, i) => (
          <div key={i} className="flex flex-col gap-2">
            <Meter label="GPU" frac={g.util / 100} text={`${g.util.toFixed(0)}%`} />
            <Meter label={t("ov.gpuMem")} frac={g.mem_used_gb / g.mem_total_gb} text={`${g.mem_used_gb} / ${g.mem_total_gb} GB`} />
          </div>
        ))}
        {!(h.gpus ?? []).length ? <div className="text-xs text-muted-foreground">{t("ov.noGpu")}</div> : null}
        <div className="text-[11px] text-muted-foreground" title={utcTitle(h.now)}>{t("ov.hostTime", { time: fmtTime(h.now) })}</div>
      </div>
    </WidgetCard>
  );
}

export function stallText(s: Session): string {
  const h = s.health ?? {};
  if (h.hang_reason) return h.hang_reason;
  return t("note.lowCpu", { cpu: (h.cpu_now || 0).toFixed(1), dur: fmtDur(h.low_for) });
}

function RunningSession({ s, now, onOpen }: { s: Session; now: number; onOpen: () => void }) {
  const stalled = s.state === "stalled";
  const h = s.health ?? {};
  const paused = s.sims.filter((x) => x.paused).length;
  return (
    <button type="button" onClick={onOpen}
      className={`block w-full border-b px-4 py-3 text-left last:border-b-0 hover:bg-muted/60 ${stalled ? "bg-amber-50 dark:bg-amber-950/40" : ""}`}>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="min-w-0 truncate font-semibold">{s.name}</span>
        <StatePill state={s.state} />
        <span className="text-xs text-muted-foreground">{s.host} · {s.project}{s.group ? ` · ${t("ov.group", { name: s.group })}` : ""}</span>
        <span className="ml-auto font-mono text-xs text-muted-foreground">{fmtDur(s.start ? now - s.start : null)}</span>
      </div>
      {stalled ? (
        <div className="mt-1 flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="mt-px h-3.5 w-3.5 shrink-0" />{stallText(s)}
        </div>
      ) : null}
      {s.progress.length ? (
        <div className="mt-2 flex flex-col gap-2">
          {s.progress.map((b) => <ProgressBar key={String(b.id)} bar={b} compact />)}
        </div>
      ) : null}
      <div className="mt-1 text-xs text-muted-foreground">
        {s.sims.length ? t("ov.sims", { n: s.sims.length, run: s.sims.length - paused }) + (paused ? t("ov.simsPaused", { n: paused }) : "") : null}
        {s.heartbeat?.sim_time_ns != null ? ` ${t("ov.simTime", { t: fmtSim(s.heartbeat.sim_time_ns) })}` : null}
        {h.eta_s != null ? ` · ${t("ov.eta", { t: fmtDur(h.eta_s) })}` : null}
        {h.output_idle && !stalled ? <span className="text-amber-700 dark:text-amber-300"> · {t("ov.noOutput", { t: fmtDur(now - (s.last_output ?? now)) })}</span> : null}
      </div>
    </button>
  );
}

const RECENT_DONE = 6;
const DAY = 86400;

function Kpis({ status, now }: { status: Status; now: number }) {
  const live = status.sessions.filter((s) => !s.archived);
  const active = live.filter(isActive);
  const sims = active.reduce((a, s) => a + s.sims.length, 0);
  const pausedSims = active.reduce((a, s) => a + s.sims.filter((x) => x.paused).length, 0);
  const stalled = active.filter((s) => s.state === "stalled");
  const failedSessions = live.filter((s) => (s.state === "failed" || s.state === "died") && now - sessionMtime(s) <= DAY).length;
  const failedCells = status.runs.filter((r) => campaignRun(r) && !r.archived && now - (r.mtime ?? 0) <= DAY)
    .reduce((a, r) => a + runFailed(r), 0);
  const hosts = status.hosts.filter((h) => h.ok && h.mem_avail_gb != null);
  const minHost = hosts.reduce<(typeof hosts)[number] | null>((m, h) => (!m || (h.mem_avail_gb ?? 0) < (m.mem_avail_gb ?? 0) ? h : m), null);
  const failed = failedSessions + failedCells;
  const queued = live.filter((s) => s.state === "queued").length;
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
      <StatTile label={t("ov.kpiSessions")} value={active.length} sub={t("ov.kpiSessionsSub", { g: new Set(active.map((s) => s.group).filter(Boolean)).size, h: new Set(active.map((s) => s.host)).size })
        + (queued ? ` · ${t("ov.queued", { n: queued })}` : "")} />
      <StatTile label={t("ov.kpiSims")} value={sims} sub={pausedSims ? t("ov.kpiSimsPaused", { n: pausedSims }) : t("ov.kpiSimsNone")} />
      <StatTile label={t("ov.kpiStalled")} value={stalled.length} tone={stalled.length ? "amber" : undefined}
        sub={stalled.length ? stalled.map((s) => s.name).join(t("common.list")) : t("ov.kpiStalledNone")} title={stalled.map((s) => `${s.name}${t("common.colon")}${stallText(s)}`).join("\n")} />
      <StatTile label={t("ov.kpiFailed")} value={failed} tone={failed ? "red" : undefined}
        sub={t("ov.kpiFailedSub", { s: failedSessions, c: failedCells })} title={t("ov.kpiFailedHint")} />
      <StatTile label={t("ov.kpiMem")} value={minHost ? `${(minHost.mem_avail_gb ?? 0).toFixed(0)} GB` : "—"}
        sub={minHost ? t("ov.kpiMemSub", { name: minHost.name, total: (minHost.mem_total_gb ?? 0).toFixed(0) }) : t("ov.kpiMemNone")} />
    </div>
  );
}

export default function OverviewPage() {
  const { status } = useStatusState();
  const navigate = useNavigate();
  if (!status) return <div className="exp-page h-full"><EmptyState title={t("common.connecting")} hint={t("common.readingStatus")} /></div>;
  const now = status.updated ?? Date.now() / 1000;
  const running = status.sessions.filter((s) => isLive(s) && !s.archived)
    .sort((a, b) => (a.state === "stalled" ? 0 : 1) - (b.state === "stalled" ? 0 : 1) || (b.start ?? 0) - (a.start ?? 0));
  const camps = status.runs.filter((r) => campaignRun(r) && !r.archived && r.progress);
  const open = camps.filter((r) => !campaignDone(r)).sort((a, b) => (b.mtime ?? 0) - (a.mtime ?? 0));
  const done = camps.filter(campaignDone).sort((a, b) => (b.mtime ?? 0) - (a.mtime ?? 0)).slice(0, RECENT_DONE);

  const campRow = (r: (typeof camps)[number]) => (
    <div key={r.id} className="border-b px-4 py-3 last:border-b-0">
      <ProgressBar bar={r.progress!} failed={runFailed(r)} compact
        label={<span className="flex min-w-0 flex-wrap items-baseline gap-x-2"><RunName run={r} projects={status.projects} withGroup />
          <span className="text-xs font-normal text-muted-foreground">{r.host}{r.kind === "sensitivity" ? ` · ${t("common.sensitivity")}` : ""}</span></span>}
        extra={
          <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
            {runFailed(r) ? <span className="text-red-600 dark:text-red-400" title={r.failures.join("\n")}>{t("progress.failed", { n: runFailed(r) })}</span> : null}
            {r.progress!.in_progress ? <span>{t("ov.inProgress", { n: r.progress!.in_progress })}</span> : null}
            <span className="ml-auto font-mono" title={utcTitle(r.mtime)}>{fmtTime(r.mtime)}</span>
            <button type="button" className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-primary hover:bg-primary/10"
              onClick={() => { plotOnly(`${r.host}|${r.project}|${r.name}`); navigate("/charts"); }}>
              <BarChart3 className="h-3.5 w-3.5" />{t("common.plot")}
            </button>
          </div>
        } />
    </div>
  );
  const card = "mb-3 break-inside-avoid";

  return (
    <div className="exp-page h-full overflow-auto p-3 sm:p-4">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-3">
        <Kpis status={status} now={now} />
        <div className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr))]">
          {status.hosts.map((h) => <HostCard key={h.name} h={h} status={status} />)}
        </div>
        <div className="gap-3 lg:columns-2">
          <WidgetCard className={card} title={t("ov.running")} expandHref="/sessions" contentClassName="p-0"
            headerRight={<span className="font-mono text-xs text-muted-foreground">{running.length}</span>}>
            {running.length ? running.map((s) => (
              <RunningSession key={s.id} s={s} now={now} onOpen={() => navigate(`/sessions?id=${encodeURIComponent(s.id)}`)} />
            )) : <EmptyState className="p-6" title={t("ov.noRunning")} hint={t("ov.noRunningHint")} />}
          </WidgetCard>
          <WidgetCard className={card} title={t("ov.campaigns")} expandHref="/results" contentClassName="p-0"
            headerRight={<span className="font-mono text-xs text-muted-foreground">{t("ov.unfinished", { n: open.length })}</span>}>
            {open.length ? open.map(campRow) : <EmptyState className="p-6" title={t("ov.noOpen")} hint={t("ov.noOpenHint")} />}
          </WidgetCard>
          <WidgetCard className={card} title={t("ov.recentDone")} expandHref="/results" contentClassName="p-0"
            headerRight={<span className="font-mono text-xs text-muted-foreground">{done.length}</span>}>
            {done.length ? done.map(campRow) : <EmptyState className="p-6" title={t("ov.noDone")} hint={t("ov.noDoneHint")} />}
          </WidgetCard>
        </div>
      </div>
    </div>
  );
}
