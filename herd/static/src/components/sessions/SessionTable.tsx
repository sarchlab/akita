import { ChevronDown, ChevronRight } from "lucide-react";
import { Fragment } from "react";
import { fmtDur, fmtSim, fmtTimeShort, utcTitle } from "../../lib/format";
import { isActive, sessionMtime } from "../../lib/derive";
import { stateLabel } from "../../lib/notify";
import { t } from "../../lib/i18n";
import type { Session } from "../../lib/types";
import ProgressBar from "../ProgressBar";
import StatePill, { ArchivedTag, Pill } from "../StatePill";

export type Item = { group: string; members: Session[] } | { session: Session };

const SEG_ORDER = ["ok", "failed", "died", "stopped", "stalled", "running", "queued"] as const;
const SEG_COLOR: Record<string, string> = {
  ok: "bg-emerald-600", failed: "bg-red-600", died: "bg-amber-700", stopped: "bg-slate-400", stalled: "bg-amber-500", running: "bg-sky-600", queued: "bg-violet-500",
};
const TEXT_COLOR: Record<string, string> = {
  ok: "text-emerald-700 dark:text-emerald-400", failed: "text-red-600 dark:text-red-400", died: "text-amber-700 dark:text-amber-400",
  stopped: "text-muted-foreground", stalled: "font-bold text-amber-600 dark:text-amber-400", running: "text-sky-700 dark:text-sky-400", queued: "text-violet-700 dark:text-violet-400",
};

const rss = (s: Session) => s.procs.reduce((a, p) => a + p.rss_mb, 0);

function ProgressCell({ s }: { s: Session }) {
  const h = s.health ?? {};
  const hb = s.heartbeat;
  if (!isActive(s)) return <span className="text-muted-foreground">—</span>;
  const paused = s.sims.filter((x) => x.paused).length;
  return (
    <div className="flex w-44 flex-col gap-1.5">
      {s.progress.slice(0, 3).map((b) => <ProgressBar key={String(b.id)} bar={b} compact />)}
      {s.progress.length > 3 ? <span className="text-[11px] text-muted-foreground">{t("st.moreBars", { n: s.progress.length - 3 })}</span> : null}
      {h.hang_basis === "heartbeat" && hb ? (
        <div className="text-[11px] text-muted-foreground">
          {fmtSim(hb.sim_time_ns)} · {h.sim_rate != null ? t("st.perMin", { r: fmtSim(h.sim_rate * 60) }) : t("st.measuring")}
          {h.eta_s != null ? ` · ${t("ov.eta", { t: fmtDur(h.eta_s) })}` : h.wg_rate === 0 ? ` · ${t("st.noWork")}` : ""}
        </div>
      ) : null}
      {s.sims.length ? (
        <div className="text-[11px] text-muted-foreground">
          {t("ov.sims", { n: s.sims.length, run: s.sims.length - paused })}{paused ? <span className="text-amber-600">{t("ov.simsPaused", { n: paused })}</span> : null}
        </div>
      ) : null}
      {!s.progress.length && !s.sims.length && !(hb && h.hang_basis === "heartbeat") ? <span className="text-muted-foreground">—</span> : null}
    </div>
  );
}

function StateCell({ s, now }: { s: Session; now: number }) {
  const h = s.health ?? {};
  const outAge = now - (s.last_output ?? sessionMtime(s));
  return (
    <div className="flex flex-col items-start gap-1">
      <span className="flex flex-wrap items-center gap-1">
        <StatePill state={s.state} exitCode={s.exit_code} />
        {s.archived ? <ArchivedTag /> : null}
      </span>
      {s.state === "stalled" ? (
        <span className="text-xs text-amber-700 dark:text-amber-300">
          {h.hang_reason || t("st.lowCpuOut", { cpu: (h.cpu_now || 0).toFixed(1), dur: fmtDur(h.low_for), out: fmtDur(outAge) })}
        </span>
      ) : h.output_idle ? (
        <span className="rounded border border-amber-500 px-1 text-[11px] text-amber-700 dark:text-amber-300">{t("ov.noOutput", { t: fmtDur(outAge) })}</span>
      ) : null}
      {isActive(s) && !s.sims.length && s.procs.length > 1 && h.hang_basis !== "heartbeat" ? (
        <span className="rounded border border-amber-500 px-1 text-[11px] text-amber-700 dark:text-amber-300" title={t("st.procsHint")}>{t("st.procs", { n: s.procs.length })}</span>
      ) : null}
    </div>
  );
}

function SessionRow({ s, now, live, selected, member, onSelect }: {
  s: Session; now: number; live: boolean; selected: boolean; member: boolean; onSelect: () => void;
}) {
  const active = isActive(s);
  const dur = s.start ? (active ? now : s.end ?? sessionMtime(s)) - s.start : null;
  const cpu = !active ? "—" : s.health?.cpu_now == null ? t("common.measuring") : s.health.cpu_now.toFixed(0) + "%";
  const last = s.last_output ?? sessionMtime(s);
  return (
    <tr onClick={onSelect}
      className={`cursor-pointer ${selected ? "herd-row-selected" : "herd-row"} ${s.state === "stalled" && !selected ? "bg-amber-50 dark:bg-amber-950/30" : ""} ${s.archived ? "opacity-60" : ""}`}>
      <td className="min-w-48">
        <div className={`flex items-center gap-1 ${member ? "pl-5" : ""}`}>
          <span className="w-4 shrink-0" />
          <span className="max-w-[15rem] truncate whitespace-nowrap font-medium" title={s.name}>{s.name}</span>
        </div>
      </td>
      <td className="whitespace-nowrap">{s.host}</td>
      <td><StateCell s={s} now={now} /></td>
      <td className="whitespace-nowrap text-muted-foreground max-2xl:hidden">{s.project}</td>
      <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden" title={utcTitle(s.start)}>{fmtTimeShort(s.start)}</td>
      <td className="whitespace-nowrap font-mono text-xs tabular-nums">{fmtDur(dur)}</td>
      {live ? <>
        <td><ProgressCell s={s} /></td>
        <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden">{cpu}</td>
        <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden">{active ? (rss(s) / 1024).toFixed(1) + " GB" : "—"}</td>
      </> : null}
      <td className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground max-md:hidden" title={utcTitle(last)}>{t("common.ago", { dur: fmtDur(now - last) })}</td>
    </tr>
  );
}

function groupSummary(members: Session[], now: number) {
  const c: Record<string, number> = { running: 0, stalled: 0, queued: 0, ok: 0, failed: 0, died: 0, stopped: 0 };
  let cpu = 0, mem = 0, eta: number | null = null, etaKnown = true, start: number | null = null, end: number | null = null, lastOut: number | null = null;
  for (const s of members) {
    c[s.state] = (c[s.state] ?? 0) + 1;
    if (s.start && (start == null || s.start < start)) start = s.start;
    const fin = isActive(s) ? now : s.end ?? sessionMtime(s);
    if (end == null || fin > end) end = fin;
    const out = s.last_output ?? sessionMtime(s);
    if (lastOut == null || out > lastOut) lastOut = out;
    if (isActive(s)) {
      cpu += s.health?.cpu_now || 0;
      mem += rss(s);
      if (s.health?.eta_s != null) eta = Math.max(eta || 0, s.health.eta_s);
      else etaKnown = false;
    }
  }
  const active = c.running + c.stalled;
  return {
    c, total: members.length, active, cpu, mem, start, end, lastOut, eta: active && etaKnown ? eta : null,
    hosts: [...new Set(members.map((m) => m.host))], projects: [...new Set(members.map((m) => m.project))],
  };
}

function GroupRow({ name, members, now, live, open, onToggle }: {
  name: string; members: Session[]; now: number; live: boolean; open: boolean; onToggle: () => void;
}) {
  const g = groupSummary(members, now);
  const st = g.c.stalled ? "stalled" : g.active ? "running" : g.c.queued ? "queued" : g.c.failed || g.c.died ? "failed" : g.c.ok ? "ok" : "stopped";
  const label = g.c.stalled ? t("st.gStalled", { n: g.c.stalled }) : g.active ? t("state.running") : g.c.queued ? t("state.queued")
    : g.c.failed || g.c.died ? t("st.gFailed") : g.c.stopped && !g.c.ok ? t("state.stopped") : g.c.stopped ? t("st.gPartly") : t("st.gAllDone");
  const done = g.c.ok + g.c.failed + g.c.died + g.c.stopped;
  const narch = members.filter((m) => m.archived).length, allArch = narch === members.length;
  const progress = (
    <div className="mt-1 w-44">
      <div className="font-mono text-xs tabular-nums">{t("st.ended", { n: done, total: g.total })}{g.active && g.eta != null ? ` · ${t("ov.eta", { t: fmtDur(g.eta) })}` : ""}</div>
      <div className="my-1 flex h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
        {SEG_ORDER.filter((s) => g.c[s]).map((s) => (
          <div key={s} className={SEG_COLOR[s]} style={{ width: `${(100 * g.c[s]) / g.total}%` }} title={`${stateLabel(s)} ${g.c[s]}`} />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-2 text-[11px]">
        {SEG_ORDER.filter((s) => g.c[s]).map((s) => <span key={s} className={TEXT_COLOR[s]}>{stateLabel(s)} {g.c[s]}</span>)}
      </div>
    </div>
  );
  const Chevron = open ? ChevronDown : ChevronRight;
  return (
    <tr onClick={onToggle} className={`herd-row cursor-pointer bg-muted/40 ${g.c.stalled ? "bg-amber-50 dark:bg-amber-950/30" : ""} ${allArch ? "opacity-60" : ""}`}>
      <td className="min-w-48">
        <div className="flex items-center gap-1">
          <Chevron className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="max-w-[15rem] truncate whitespace-nowrap font-semibold" title={name}>{name}</span>
          {allArch ? <ArchivedTag /> : null}
        </div>
        <div className="pl-5 text-xs font-normal text-muted-foreground">{t("st.gCount", { n: g.total })}{narch && !allArch ? ` · ${t("st.gArchived", { n: narch })}` : ""}</div>
      </td>
      <td><div className="max-w-[8rem] truncate whitespace-nowrap" title={g.hosts.join(", ")}>{g.hosts.join(", ")}</div></td>
      <td><Pill kind={st}>{label}</Pill>{live ? null : progress}</td>
      <td className="max-2xl:hidden"><div className="max-w-[8rem] truncate whitespace-nowrap text-muted-foreground" title={g.projects.join(", ")}>{g.projects.join(", ")}</div></td>
      <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden" title={utcTitle(g.start)}>{fmtTimeShort(g.start)}</td>
      <td className="whitespace-nowrap font-mono text-xs tabular-nums" title={g.active ? t("st.spanNow") : t("st.spanEnd")}>
        {fmtDur(g.start && g.end ? g.end - g.start : null)}
      </td>
      {live ? <>
        <td>{progress}</td>
        <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden">{g.active ? g.cpu.toFixed(0) + "%" : "—"}</td>
        <td className="whitespace-nowrap font-mono text-xs tabular-nums max-md:hidden">{g.active ? (g.mem / 1024).toFixed(1) + " GB" : "—"}</td>
      </> : null}
      <td className="whitespace-nowrap font-mono text-xs tabular-nums text-muted-foreground max-md:hidden" title={utcTitle(g.lastOut)}>
        {g.lastOut ? t("common.ago", { dur: fmtDur(now - g.lastOut) }) : "—"}
      </td>
    </tr>
  );
}

const rank = (s: Session) => (s.state === "stalled" ? 2 : s.state === "running" || s.state === "queued" ? 1 : 0);

export default function SessionTable({ items, now, selected, openGroups, onSelect, onToggleGroup }: {
  items: Item[]; now: number; selected: string | null; openGroups: Set<string>;
  onSelect: (id: string) => void; onToggleGroup: (g: string) => void;
}) {
  const live = items.some((it) => ("group" in it ? it.members : [it.session]).some(isActive));
  return (
    <table className="herd-table">
      <thead className="sticky top-0 z-[1] bg-card">
        <tr>
          <th>{t("st.session")}</th><th>{t("common.host")}</th><th>{t("sd.state")}</th><th className="max-2xl:hidden">{t("sd.project")}</th>
          <th className="max-md:hidden">{t("sd.start")}</th><th>{t("sd.elapsed")}</th>
          {live ? <><th>{t("sd.progress")}</th><th className="max-md:hidden">CPU</th><th className="max-md:hidden">{t("ov.mem")}</th></> : null}
          <th className="max-md:hidden">{t("st.lastOutput")}</th>
        </tr>
      </thead>
      <tbody>
        {items.map((it) => {
          if ("session" in it) {
            return <SessionRow key={it.session.id} s={it.session} now={now} live={live} member={false}
              selected={selected === it.session.id} onSelect={() => onSelect(it.session.id)} />;
          }
          const open = openGroups.has(it.group);
          const sorted = [...it.members].sort((a, b) => Number(a.archived) - Number(b.archived) || rank(b) - rank(a) || a.name.localeCompare(b.name));
          return (
            <Fragment key={"g:" + it.group}>
              <GroupRow name={it.group} members={it.members} now={now} live={live} open={open} onToggle={() => onToggleGroup(it.group)} />
              {open ? sorted.map((s) => (
                <SessionRow key={s.id} s={s} now={now} live={live} member selected={selected === s.id} onSelect={() => onSelect(s.id)} />
              )) : null}
            </Fragment>
          );
        })}
      </tbody>
    </table>
  );
}

export function buildItems(rows: Session[], grouped: boolean): { items: Item[]; groups: number } {
  rows = [...rows].sort((a, b) => rank(b) - rank(a) || sessionMtime(b) - sessionMtime(a));
  const items: (Item & { rank: number; mtime: number; arch: boolean })[] = [];
  const groups = new Map<string, Item & { group: string; members: Session[]; rank: number; mtime: number; arch: boolean }>();
  for (const s of rows) {
    if (grouped && s.group) {
      let g = groups.get(s.group);
      if (!g) {
        g = { group: s.group, members: [], rank: 0, mtime: 0, arch: false };
        groups.set(s.group, g);
        items.push(g);
      }
      g.members.push(s);
      g.rank = Math.max(g.rank, rank(s));
      g.mtime = Math.max(g.mtime, sessionMtime(s));
    } else items.push({ session: s, rank: rank(s), mtime: sessionMtime(s), arch: s.archived });
  }
  for (const g of groups.values()) g.arch = g.members.every((m) => m.archived);
  items.sort((a, b) => Number(a.arch) - Number(b.arch) || b.rank - a.rank || b.mtime - a.mtime);
  return { items, groups: groups.size };
}
