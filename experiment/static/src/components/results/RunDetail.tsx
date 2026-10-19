import { BarChart3 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { campaignRun, runFailed } from "../../lib/derive";
import { runCache, useCache, type RunDetail as Detail } from "../../lib/fetchCache";
import { fmtNum, fmtTime, utcTitle } from "../../lib/format";
import { groupLabel, namingOf, fieldLabel, valueLabel, type Projects } from "../../lib/naming";
import { csvRows, flagsOf, isOutputFlag } from "../../lib/summary";
import { t } from "../../lib/i18n";
import { runKey, type Run } from "../../lib/types";
import ProgressBar from "../ProgressBar";
import { Section } from "../Section";
import PropertyList from "../PropertyList";
import { ArchivedTag } from "../StatePill";
import { Input } from "../ui/input";
import RunStatus from "./RunStatus";


function SummaryTable({ text }: { text: string }) {
  const rows = csvRows(text);
  if (!rows.length) return null;
  const [head, ...body] = rows;
  return (
    <div className="overflow-x-auto">
      <table className="exp-table exp-table-sm">
        <thead><tr>{head.map((h, i) => <th key={i}>{h}</th>)}</tr></thead>
        <tbody>{body.map((r, i) => <tr key={i}>{r.map((c, j) => <td key={j} className="break-all font-mono">{c}</td>)}</tr>)}</tbody>
      </table>
    </div>
  );
}

function StatsTable({ metrics }: { metrics: NonNullable<Detail["metrics"]> }) {
  const [filter, setFilter] = useState("");
  const keys = useMemo(() => Object.keys(metrics.agg).sort(), [metrics]);
  const f = filter.toLowerCase();
  const shown = keys.filter((k) => !f || k.toLowerCase().includes(f)).slice(0, 400);
  const nf = Object.entries(metrics.nonfinite || {});
  const nfCount = nf.reduce((a, [, n]) => a + n, 0);
  return (
    <div>
      {nfCount ? (
        <div className="mb-1 text-xs text-muted-foreground" title={nf.map(([k, n]) => `${k}${t("common.colon")}${n}`).join("\n")}>
          {t("rd.nonfinite", { n: nfCount, list: nf.slice(0, 3).map(([k]) => k).join(t("common.list")), more: nf.length > 3 ? t("gmean.more") : "" })}
        </div>
      ) : null}
      <Input className="mb-2 h-8" value={filter} placeholder={t("rd.filter")} onChange={(e) => setFilter(e.target.value)} />
      <div className="max-h-96 overflow-auto rounded border">
        <table className="exp-table exp-table-sm">
          <thead className="sticky top-0 bg-card"><tr><th>{t("rd.kind")}</th><th>{t("rd.stat")}</th><th>{t("rd.sum")}</th><th>{t("rd.n")}</th><th>{t("rd.min")}</th><th>{t("rd.max")}</th></tr></thead>
          <tbody>{shown.map((k) => {
            const [kind, metric] = k.split("|"), a = metrics.agg[k];
            return (
              <tr key={k}>
                <td className="break-all font-mono">{kind}</td><td className="break-all font-mono">{metric}</td>
                <td className="font-mono tabular-nums">{fmtNum(a.sum)}</td><td className="font-mono tabular-nums">{a.n}</td>
                <td className="font-mono tabular-nums">{fmtNum(a.min)}</td><td className="font-mono tabular-nums">{fmtNum(a.max)}</td>
              </tr>
            );
          })}</tbody>
        </table>
      </div>
    </div>
  );
}

/** The old page's result detail (/api/v1/run): effective flags, statistics digest, meta.json. */
function SingleRunDetail({ k }: { k: string }) {
  const cache = useCache(runCache);
  const e = cache[k];
  useEffect(() => { if (!e || (e.error && !e.loading && Date.now() - e.at > 10000)) void runCache.load(k); }, [k, e]);
  if (!e || (e.loading && !e.data)) return <Section title={t("rd.cfgStats")}><div className="text-xs text-muted-foreground">{t("rd.reading")}</div></Section>;
  if (e.error && !e.data) return <Section title={t("rd.cfgStats")}><div className="text-xs text-red-600">{t("rd.failed", { error: e.error })}</div></Section>;
  const d = e.data as Detail;
  const flags = flagsOf(d);
  const cmdline = new Set(d.config?.command_line || []);
  const names = Object.keys(flags).filter((n) => !isOutputFlag(n)).sort();
  return (
    <>
      <Section title={t("rd.config")} subtitle={d.config ? t("rd.configFiles", { files: (d.config.files || []).map((f) => f.split("/").pop()).join(", ") || t("rd.none") }) : t("rd.fromMeta")}>
        <div className="max-h-96 overflow-auto rounded border">
          <table className="exp-table exp-table-sm"><tbody>
            {names.map((n) => (
              <tr key={n}><td className="break-all font-mono">-{n}</td><td className="break-all font-mono">{flags[n]}</td>
                <td className="whitespace-nowrap text-muted-foreground">{cmdline.has(n) ? t("rd.cmdline") : ""}</td></tr>
            ))}
          </tbody></table>
        </div>
      </Section>
      {d.metrics ? (
        <Section title={t("rd.statsTitle")} subtitle={t("rd.statsSub", { n: d.metrics.count, k: Object.keys(d.metrics.agg).length })}>
          <StatsTable metrics={d.metrics} />
        </Section>
      ) : null}
      {d.meta ? <Section title="meta.json"><pre className="exp-pre">{JSON.stringify(d.meta, null, 2)}</pre></Section> : null}
    </>
  );
}

export default function RunDetail({ run, projects, cmpIndex, onCmp, onPlot, onPlotAdd, plotIndex }: {
  run: Run; projects: Projects; cmpIndex: number; plotIndex: number;
  onCmp: () => void; onPlot: () => void; onPlotAdd: () => void;
}) {
  const naming = namingOf(projects, run.project);
  const key = runKey(run);
  const fields = run.fields;
  return (
    <div>
      <section className="border-b p-4">
        {fields && naming ? <div className="text-sm font-semibold">{groupLabel(naming, fields)}</div> : null}
        <div className="mt-0.5 break-all font-mono text-xs">{run.name}{run.archived ? <ArchivedTag /> : null}</div>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>{run.host} · {run.project}</span>
          <RunStatus run={run} />
          <span title={utcTitle(run.mtime)}>{t("rd.updated", { time: fmtTime(run.mtime) })}</span>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <label className="inline-flex items-center gap-1.5 text-xs font-medium">
            <input type="checkbox" className="h-4 w-4 accent-primary" checked={cmpIndex >= 0} onChange={onCmp} />
            {t("rl.cmp")}{cmpIndex >= 0 ? ` ${"ABCD"[cmpIndex]}` : ""}
          </label>
          {campaignRun(run) ? <>
            <button type="button" className="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs text-primary hover:bg-primary/10" onClick={onPlot}>
              <BarChart3 className="h-3.5 w-3.5" />{t("common.plot")}
            </button>
            <button type="button" className={`rounded border px-2 py-0.5 text-xs ${plotIndex >= 0 ? "border-primary bg-primary text-primary-foreground" : "text-primary hover:bg-primary/10"}`} onClick={onPlotAdd}>
              {plotIndex >= 0 ? t("rd.inChart", { l: "ABCD"[plotIndex] }) : t("rd.addChart")}
            </button>
          </> : null}
        </div>
        {fields && naming ? (
          <dl className="mt-3 grid grid-cols-[5rem_minmax(0,1fr)] gap-y-1 text-sm">
            {naming.fields.filter((f) => fields[f.name] != null).map((f) => (
              <div key={f.name} className="contents">
                <dt className="text-muted-foreground">{fieldLabel(naming, f.name)}</dt>
                <dd className="font-mono text-xs">{valueLabel(naming, f.name, fields[f.name])}
                  {valueLabel(naming, f.name, fields[f.name]) !== fields[f.name] ? <span className="ml-1 text-muted-foreground">{fields[f.name]}</span> : null}</dd>
              </div>
            ))}
          </dl>
        ) : null}
      </section>
      {run.progress ? (
        <Section title={t("ov.campaigns")}>
          <ProgressBar bar={run.progress} failed={runFailed(run)} label={t("rd.endedTotal")} />
          {run.failures.length ? (
            <div className="mt-2 text-xs">
              <div className="font-medium text-red-600 dark:text-red-400">{t("rd.failedCells")}</div>
              <div className="mt-1 break-all font-mono text-muted-foreground">{run.failures.join("  ")}</div>
            </div>
          ) : null}
        </Section>
      ) : null}
      <section className="border-b p-4">
        <PropertyList entries={run.properties} title={t("rd.props")} source={t("rd.propsSrc")} />
      </section>
      {run.deviation?.length ? (
        <Section title={t("rd.deviation")} subtitle={t("rl.devHint")}>
          <div className="flex flex-wrap gap-1">{run.deviation.map((d) => <span key={d} className="rounded border border-amber-500 px-1 font-mono text-[11px] text-amber-700 dark:text-amber-300">{d}</span>)}</div>
        </Section>
      ) : null}
      {run.metrics && Object.keys(run.metrics).length ? (
        <Section title={t("rd.metrics")}>
          <table className="exp-table exp-table-sm"><tbody>
            {Object.entries(run.metrics).map(([k, v]) => <tr key={k}><td className="font-mono">{k}</td><td className="font-mono tabular-nums">{fmtNum(v)}</td></tr>)}
          </tbody></table>
        </Section>
      ) : null}
      {run.summary ? <Section title="summary.csv"><SummaryTable text={run.summary} /></Section> : null}
      {run.kind === "single" ? <SingleRunDetail k={key} /> : null}
      {run.files?.length ? (
        <Section title={t("rd.files")} subtitle={String(run.files.length)}>
          <div className="max-h-60 overflow-auto break-all font-mono text-[11px] leading-relaxed text-muted-foreground">{run.files.join("  ")}</div>
        </Section>
      ) : null}
    </div>
  );
}
