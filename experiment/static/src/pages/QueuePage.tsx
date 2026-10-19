import Meter from "../components/Meter";
import StatePill, { Pill } from "../components/StatePill";
import WidgetCard from "../components/WidgetCard";
import { EmptyState } from "../components/Section";
import { hostType } from "../components/Layout";
import { fmtDur, fmtTimeShort, utcTitle } from "../lib/format";
import { t } from "../lib/i18n";
import { useStatusState } from "../lib/status";
import type { Host, QueueStatus, Task } from "../lib/types";

const DONE_SHOWN = 15;

const mem = (gb?: number) => (gb ? `${gb} GB` : "—");

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="border-t">
      <div className="bg-muted/40 px-4 py-1.5 text-xs font-semibold text-muted-foreground">{title}</div>
      {children}
    </div>
  );
}

function PendingTable({ q }: { q: QueueStatus }) {
  const rows = q.pending ?? [];
  if (!rows.length) return <EmptyState className="p-3" title={t("q.noPending")} />;
  return (
    <table className="exp-table exp-table-sm w-full">
      <thead><tr>
        <th>{t("q.colOrder")}</th><th>{t("q.colName")}</th><th className="max-md:hidden">{t("q.colProject")}</th>
        <th title={t("q.prioHint")}>{t("q.colPrio")}</th><th>{t("q.colMem")}</th><th>{t("q.colSince")}</th><th>{t("q.colInfo")}</th>
      </tr></thead>
      <tbody>
        {rows.map((x, i) => (
          <tr key={`${x.project}/${x.name}`} className="exp-row">
            <td className="font-mono text-xs tabular-nums">{i + 1}</td>
            <td className="break-all font-medium" title={x.cmd}>{x.name}</td>
            <td className="text-muted-foreground max-md:hidden">{x.project}</td>
            <td className="font-mono text-xs tabular-nums">{x.priority}</td>
            <td className="whitespace-nowrap font-mono text-xs">{mem(x.mem_gb)}</td>
            <td className="whitespace-nowrap font-mono text-xs" title={utcTitle(x.submitted)}>{x.submitted ? fmtDur(q.now - x.submitted) : "—"}</td>
            <td className="text-xs text-muted-foreground">
              {x.avoid_hosts?.length ? t("q.avoid", { hosts: x.avoid_hosts.join(", ") }) : ""}
              {x.attempts?.length ? ` ${t("q.attempt", { n: x.attempts.length + 1 })}` : ""}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function RunningTable({ q }: { q: QueueStatus }) {
  const rows = q.running ?? [];
  if (!rows.length) return <EmptyState className="p-3" title={t("q.noRunning")} />;
  return (
    <table className="exp-table exp-table-sm w-full">
      <thead><tr>
        <th>{t("q.colName")}</th><th>{t("q.colState")}</th><th className="max-md:hidden">{t("q.colProject")}</th>
        <th>{t("q.colPrio")}</th><th>{t("q.colMem")}</th><th>{t("q.colSince")}</th><th>{t("q.colInfo")}</th>
      </tr></thead>
      <tbody>
        {rows.map((x) => (
          <tr key={`${x.project}/${x.name}`} className={x.paused ? "bg-amber-50 dark:bg-amber-950/30" : "exp-row"}>
            <td className="break-all font-medium" title={x.cmd}>{x.name}</td>
            <td>{x.paused ? <Pill kind="stalled">{t("q.paused")}</Pill> : <StatePill state="running" />}</td>
            <td className="text-muted-foreground max-md:hidden">{x.project}</td>
            <td className="font-mono text-xs tabular-nums">{x.priority}</td>
            <td className="whitespace-nowrap font-mono text-xs">{mem(x.mem_gb)}</td>
            <td className="whitespace-nowrap font-mono text-xs" title={utcTitle(x.started)}>{x.started ? fmtDur(q.now - x.started) : "—"}</td>
            <td className="whitespace-nowrap text-xs text-muted-foreground">
              {x.pgid ? `pid ${x.pgid}` : ""}
              {x.paused && x.paused_at ? ` · ${t("q.pausedFor", { t: fmtDur(q.now - x.paused_at) })}` : ""}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function doneInfo(x: Task): string {
  const parts: string[] = [];
  if (x.signal) parts.push(x.signal);
  if (x.error) parts.push(x.error);
  if (x.retried) parts.push(t("q.retried", { host: x.retried }));
  else if (x.retryable) parts.push(t("q.retryable"));
  if ((x.attempts?.length ?? 0) > 1) parts.push(t("q.attempt", { n: x.attempts!.length }));
  if (x.collected) parts.push(t("q.collected"));
  return parts.join(" · ");
}

function DoneTable({ q }: { q: QueueStatus }) {
  const all = q.done ?? [];
  if (!all.length) return <EmptyState className="p-3" title={t("q.noDone")} />;
  const rows = all.slice(0, DONE_SHOWN);
  return (
    <>
      <table className="exp-table exp-table-sm w-full">
        <thead><tr>
          <th>{t("q.colName")}</th><th>{t("q.colState")}</th><th className="max-md:hidden">{t("q.colProject")}</th>
          <th>{t("q.colEnded")}</th><th>{t("q.colInfo")}</th>
        </tr></thead>
        <tbody>
          {rows.map((x) => (
            <tr key={`${x.project}/${x.name}`} className="exp-row">
              <td className="break-all font-medium" title={x.cmd}>{x.name}</td>
              <td><StatePill state={x.state || "unknown"} exitCode={x.exit} /></td>
              <td className="text-muted-foreground max-md:hidden">{x.project}</td>
              <td className="whitespace-nowrap font-mono text-xs" title={utcTitle(x.ended)}>{fmtTimeShort(x.ended)}</td>
              <td className={`text-xs ${x.retryable && !x.retried ? "text-amber-700 dark:text-amber-300" : "text-muted-foreground"}`}>{doneInfo(x)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {all.length > rows.length ? <div className="px-4 py-1.5 text-xs text-muted-foreground">{t("q.moreDone", { n: all.length - rows.length })}</div> : null}
    </>
  );
}

function QueueCard({ h, q }: { h: Host; q: QueueStatus }) {
  const a = q.agent;
  const alive = !!a && q.alive;
  const used = a ? Math.max(a.own, a.external) : (q.running ?? []).length;
  return (
    <WidgetCard contentClassName="p-0"
      title={<>{h.name} <span className="text-xs font-normal text-muted-foreground">{hostType(h)} · {q.root}</span></>}
      headerRight={a ? (
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="hidden sm:inline">{t("q.heartbeat", { pid: a.pid, age: fmtDur(q.now - a.heartbeat) })}</span>
          <Pill kind={alive ? "ok" : "failed"}>{alive ? t("q.agentUp") : t("q.agentDown")}</Pill>
        </span>
      ) : <Pill kind="unknown">{t("q.agentNever")}</Pill>}>
      {a ? (
        <div className="flex flex-col gap-2 px-4 pb-3">
          <div title={t("q.slotsHint")}>
            <Meter label={t("q.slots")} frac={a.slots ? used / a.slots : 0}
              text={t("q.slotsText", { used, slots: a.slots, own: a.own, ext: a.external })} />
          </div>
          <div className="text-xs">
            <span className="font-mono">{t("q.memText", { avail: a.mem_avail_gb.toFixed(1) })}</span>
            <span className="text-muted-foreground"> · {t("q.memRules", { reserve: a.mem_reserve_gb, pause: a.mem_pause_gb, resume: a.mem_resume_gb })}</span>
          </div>
          {a.error ? <div className="text-xs text-red-600 dark:text-red-400">{t("q.agentError", { error: a.error })}</div> : null}
        </div>
      ) : null}
      <Section title={t("q.running", { n: (q.running ?? []).length })}><RunningTable q={q} /></Section>
      <Section title={t("q.pending", { n: (q.pending ?? []).length })}><PendingTable q={q} /></Section>
      <Section title={t("q.done", { n: (q.done ?? []).length })}><DoneTable q={q} /></Section>
    </WidgetCard>
  );
}

export default function QueuePage() {
  const { status } = useStatusState();
  if (!status) return <div className="exp-page h-full"><EmptyState title={t("common.connecting")} hint={t("common.readingStatus")} /></div>;
  const withQueue = status.hosts.filter((h) => h.queue);
  const without = status.hosts.filter((h) => !h.queue).map((h) => h.name);
  const activity = [...(status.activity ?? [])].reverse();
  return (
    <div className="exp-page h-full overflow-auto p-3 sm:p-4">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-3">
        {withQueue.length ? withQueue.map((h) => <QueueCard key={h.name} h={h} q={h.queue!} />)
          : <EmptyState title={t("q.noQueues")} hint={t("q.noQueuesHint")} />}
        {withQueue.length && without.length ? <div className="text-xs text-muted-foreground">{t("q.noQueueHosts", { hosts: without.join(", ") })}</div> : null}
        {activity.length ? (
          <WidgetCard title={t("q.activity")} headerRight={<span className="text-xs text-muted-foreground">{t("q.activityHint")}</span>}>
            <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all font-mono text-xs">{activity.join("\n")}</pre>
          </WidgetCard>
        ) : null}
      </div>
    </div>
  );
}
