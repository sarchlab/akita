import { Copy } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import { isActive, runFailed } from "../../lib/derive";
import { t } from "../../lib/i18n";
import { fmtDur, fmtSim, fmtSize, fmtTime, utcTitle } from "../../lib/format";
import type { Run, Session, Sim } from "../../lib/types";
import ProgressBar from "../ProgressBar";
import { Section } from "../Section";
import StatePill, { ArchivedTag, Pill } from "../StatePill";


export function CopyButton({ text }: { text: string }) {
  const [done, setDone] = useState(false);
  return (
    <button type="button" className="ml-2 inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-[11px] font-normal text-muted-foreground hover:bg-muted"
      onClick={() => { void navigator.clipboard?.writeText(text).then(() => { setDone(true); window.setTimeout(() => setDone(false), 1500); }); }}>
      <Copy className="h-3 w-3" />{done ? t("common.copied") : t("common.copy")}
    </button>
  );
}

/** The log tail; follows new lines unless the user scrolled up. */
function LogTail({ id, lines }: { id: string; lines: string[] }) {
  const ref = useRef<HTMLPreElement>(null);
  const follow = useRef(true);
  const lastId = useRef(id);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (lastId.current !== id) { follow.current = true; lastId.current = id; }
    if (follow.current) el.scrollTop = el.scrollHeight;
  }, [id, lines]);
  return (
    <pre ref={ref} className="exp-pre max-h-[28rem]"
      onScroll={(e) => { const el = e.currentTarget; follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 4; }}>
      {lines.join("\n") || t("sd.emptyLog")}
    </pre>
  );
}

/** Simulator processes grouped by campaign (the column would repeat one long name). */
function simGroups(sims: Sim[]): [string, Sim[]][] {
  const m = new Map<string, Sim[]>();
  for (const x of sims) m.set(x.campaign || "", [...(m.get(x.campaign || "") ?? []), x]);
  return [...m.entries()];
}

export default function SessionDetail({ s, runs, now }: { s: Session; runs: Run[]; now: number }) {
  const h = s.health ?? {};
  const hb = s.heartbeat;
  const campNames = new Set(s.sims.map((x) => x.campaign).filter(Boolean));
  const camps = runs.filter((r) => r.host === s.host && r.project === s.project && campNames.has(r.name) && r.progress);
  return (
    <div>
      <section className="border-b p-4">
        <div className="flex flex-wrap items-center gap-2">
          <div className="min-w-0 break-all text-sm font-semibold">{s.name}</div>
          <StatePill state={s.state} exitCode={s.exit_code} />
          {s.archived ? <ArchivedTag /> : null}
        </div>
        <dl className="mt-3 grid grid-cols-[5rem_minmax(0,1fr)] gap-y-1.5 text-sm">
          <dt className="text-muted-foreground">{t("common.host")}</dt><dd className="font-mono text-xs">{s.host}</dd>
          <dt className="text-muted-foreground">{t("sd.project")}</dt><dd className="font-mono text-xs">{s.project}</dd>
          {s.group ? <><dt className="text-muted-foreground">{t("sd.group")}</dt><dd className="font-mono text-xs">{s.group}</dd></> : null}
          {s.job ? <><dt className="text-muted-foreground">{t("sd.job")}</dt><dd className="font-mono text-xs">{s.job}</dd></> : null}
          <dt className="text-muted-foreground">{t("sd.start")}</dt><dd className="font-mono text-xs" title={utcTitle(s.start)}>{fmtTime(s.start)}</dd>
          {s.end ? <><dt className="text-muted-foreground">{t("sd.end")}</dt><dd className="font-mono text-xs" title={utcTitle(s.end)}>{fmtTime(s.end)}</dd></> : null}
          <dt className="text-muted-foreground">{t("sd.elapsed")}</dt>
          <dd className="font-mono text-xs">{fmtDur(s.start ? (isActive(s) ? now : s.end ?? now) - s.start : null)}</dd>
          {s.exit_code != null ? <><dt className="text-muted-foreground">{t("sd.exit")}</dt><dd className="font-mono text-xs">{s.exit_code}</dd></> : null}
        </dl>
        {s.state === "stalled" ? (
          <div className="mt-3 rounded border border-amber-400 bg-amber-50 p-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
            {t("sd.stalled")}{h.hang_reason || t("note.lowCpu", { cpu: (h.cpu_now || 0).toFixed(1), dur: fmtDur(h.low_for) })}
          </div>
        ) : null}
        {s.archived ? (
          <div className="mt-3 text-xs text-muted-foreground">
            {t("sd.archived", { project: s.project, name: s.name })}<span className="font-mono">akita exp unarchive {s.host} {s.name}</span>
          </div>
        ) : null}
      </section>

      {s.progress.length ? (
        <Section title={t("sd.progress")}>
          <div className="flex flex-col gap-4">{s.progress.map((b) => <ProgressBar key={String(b.id)} bar={b} />)}</div>
        </Section>
      ) : null}

      {s.sims.length ? (
        <Section title={t("sd.sims", { n: s.sims.length })} subtitle={t("sd.simsHint")}>
          {simGroups(s.sims).map(([camp, sims]) => (
            <div key={camp} className="mb-2 overflow-x-auto">
              {camp ? <div className="break-all font-mono text-[11px] text-muted-foreground">{camp}</div> : null}
              <table className="exp-table exp-table-sm exp-table-tight">
                <thead><tr><th>{t("sd.label")}</th><th>{t("sd.config")}</th><th>{t("sd.state")}</th><th className="text-right">{t("sd.etime")}</th><th className="text-right">CPU</th><th className="text-right">{t("ov.mem")}</th><th className="text-right">PID</th><th></th></tr></thead>
                <tbody>
                  {sims.map((x) => (
                    <tr key={x.pid} className={x.paused ? "text-muted-foreground" : ""}>
                      <td className="max-w-24 truncate" title={x.bench}>{x.bench}</td>
                      <td className="max-w-24 truncate font-mono" title={x.config}>{x.config || "—"}</td>
                      <td><Pill kind={x.paused ? "stalled" : "running"}>{x.paused ? t("sd.paused") : t("sd.run")}</Pill></td>
                      <td className="whitespace-nowrap text-right font-mono tabular-nums">{fmtDur(x.etime)}</td>
                      <td className="text-right font-mono tabular-nums">{x.cpu.toFixed(0)}%</td>
                      <td className="whitespace-nowrap text-right font-mono tabular-nums">{(x.rss_mb / 1024).toFixed(1)}G</td>
                      <td className="text-right font-mono tabular-nums text-muted-foreground">{x.pid}</td>
                      <td>{x.monitor ? <a className="text-primary hover:underline" href={x.monitor} target="_blank" rel="noreferrer">{t("sd.monitor")}</a> : null}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
          {camps.length ? (
            <div className="mt-3 flex flex-col gap-3">
              <div className="text-xs font-medium text-muted-foreground">{t("sd.related")}</div>
              {camps.map((r) => <ProgressBar key={r.id} bar={r.progress!} failed={runFailed(r)} compact label={<span className="font-mono text-xs">{r.name}</span>} />)}
            </div>
          ) : null}
        </Section>
      ) : null}

      <Section title={t("sd.cmd")} right={s.cmd ? <CopyButton text={s.cmd} /> : null}>
        {s.cmd ? <pre className="exp-pre max-h-40">{s.cmd}</pre>
          : <span className="text-xs text-muted-foreground">{t("sd.noCmd")}</span>}
      </Section>

      {isActive(s) && h.hang_basis === "heartbeat" && hb ? (
        <Section title={t("sd.hb")} subtitle={t("sd.hbHint")}>
          <table className="exp-table exp-table-sm"><tbody>
            <tr><td>{t("sd.simTime")}</td><td className="font-mono">{fmtSim(hb.sim_time_ns)}</td>
              <td className="text-muted-foreground">{t("sd.lastAdvance", { t: fmtDur(h.sim_still_for) })}{h.sim_rate != null ? ` · ${t("sd.rate", { r: fmtSim(h.sim_rate * 60) })}` : ""}</td></tr>
            {hb.wgs_completed != null ? <tr><td>{t("sd.work")}</td><td className="font-mono">{hb.wgs_completed}</td>
              <td className="text-muted-foreground">{t("sd.lastIncrease", { t: fmtDur(h.work_still_for) })}{h.eta_s != null ? ` · ${t("ov.eta", { t: fmtDur(h.eta_s) })}` : ""}</td></tr> : null}
            <tr><td>{t("sd.hbRow")}</td><td className="font-mono">#{hb.seq}</td>
              <td className="text-muted-foreground">{t("sd.written", { t: fmtDur(h.heartbeat_age) })} · PID {hb.pid}</td></tr>
          </tbody></table>
        </Section>
      ) : isActive(s) ? (
        <Section title={t("sd.health")}>
          <div className="text-xs text-muted-foreground">
            {t("sd.noHb", { cpu: h.cpu_now == null ? t("common.measuring") : `${h.cpu_now.toFixed(0)}%` })}
            {h.low_for ? ` · ${t("sd.lowFor", { t: fmtDur(h.low_for) })}` : ""}{h.output_idle ? ` · ${t("sd.idle")}` : ""}
          </div>
        </Section>
      ) : null}

      {isActive(s) && !s.sims.length ? (
        <Section title={t("sd.procs")}>
          {s.procs.length ? (
            <div className="overflow-x-auto">
              <table className="exp-table exp-table-sm">
                <thead><tr><th>PID</th><th>{t("sd.state")}</th><th>{t("sd.avgCpu")}</th><th>{t("ov.mem")}</th><th>{t("sd.etime")}</th><th>{t("sd.command")}</th></tr></thead>
                <tbody>{s.procs.map((p) => (
                  <tr key={p.pid}>
                    <td className="font-mono">{p.pid}</td><td className="font-mono">{p.stat}</td>
                    <td className="font-mono">{p.cpu.toFixed(0)}%</td><td className="whitespace-nowrap font-mono">{(p.rss_mb / 1024).toFixed(2)} GB</td>
                    <td className="whitespace-nowrap font-mono">{fmtDur(p.etime)}</td><td className="break-all font-mono">{p.args}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          ) : <span className="text-xs text-muted-foreground">{t("sd.noProcs")}</span>}
        </Section>
      ) : null}

      <Section title={t("sd.tail", { n: s.tail.length })} subtitle={s.log_size != null ? t("sd.logSize", { size: fmtSize(s.log_size) }) : undefined}>
        <LogTail id={s.id} lines={s.tail} />
      </Section>
    </div>
  );
}
