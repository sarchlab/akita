import { Link2, X } from "lucide-react";
import { useEffect, useState } from "react";
import { runCache, useCache, type RunDetail } from "../../lib/fetchCache";
import { fmtNum, fmtSim } from "../../lib/format";
import { setPrefs } from "../../lib/prefs";
import { flagsOf, isOutputFlag, ratio, statDiffs, summaryValues } from "../../lib/summary";
import { t } from "../../lib/i18n";
import { keyParts } from "../../lib/types";
import { Input } from "../ui/input";
import { SectionHeader } from "../Section";

const L = "ABCD";

function Ratio({ v, base }: { v: number | null | undefined; base: number | null | undefined }) {
  const r = ratio(v, base);
  if (!r) return null;
  const cls = r.cls === "eq" ? "text-muted-foreground" : r.cls === "up" ? "text-red-600 dark:text-red-400" : "text-emerald-700 dark:text-emerald-400";
  return <span className={`ml-1 text-[11px] ${cls}`}>{r.text}</span>;
}

function Banner({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return (
    <div className={`rounded px-3 py-2 text-sm font-semibold ${ok ? "bg-emerald-50 text-emerald-800 dark:bg-emerald-950/50 dark:text-emerald-300" : "bg-amber-50 text-amber-800 dark:bg-amber-950/50 dark:text-amber-300"}`}>
      {children}
    </div>
  );
}

function Body({ ds }: { ds: RunDetail[] }) {
  const [filter, setFilter] = useState("");
  const DEFAULT = t("rc.default");
  const head = <tr><th></th>{ds.map((_, i) => <th key={i}>{L[i]}</th>)}</tr>;
  const sv = ds.map((d) => summaryValues(d.summary));
  const labels = [...new Set(sv.flat().map((x) => x.label))];
  const fs = ds.map(flagsOf);
  const names = [...new Set(fs.flatMap((f) => Object.keys(f)))].filter((n) => !isOutputFlag(n)).sort();
  const diffNames = names.filter((n) => new Set(fs.map((f) => (f[n] === undefined ? DEFAULT : f[n]))).size > 1);
  const ms = ds.map((d) => d.metrics);
  let stats: React.ReactNode;
  if (ms.some((m) => !m)) stats = <div className="text-xs text-muted-foreground">{t("rc.noStats")}</div>;
  else {
    const mm = ms as NonNullable<RunDetail["metrics"]>[];
    if (mm.every((m) => m.fingerprint === mm[0].fingerprint)) {
      stats = <Banner ok>{t("rc.statsSame", { n: mm[0].count })}</Banner>;
    } else {
      const { rows, keys } = statDiffs(mm);
      const f = filter.toLowerCase();
      const shown = rows.filter((r) => !f || r.k.toLowerCase().includes(f)).slice(0, 300);
      stats = (
        <>
          <Banner ok={false}>{t("rc.statsDiffer", { counts: mm.map((m, i) => `${L[i]} ${t("common.items", { n: m.count })}`).join(t("common.sep")), keys, n: rows.length })}</Banner>
          <Input className="my-2 h-8 max-w-xs" value={filter} placeholder={t("rc.filter")} onChange={(e) => setFilter(e.target.value)} />
          <div className="max-h-96 overflow-auto rounded border">
            <table className="exp-table exp-table-sm">
              <thead className="sticky top-0 bg-card"><tr><th>{t("rc.kindStat")}</th>{ds.map((_, i) => <th key={i}>{L[i]}</th>)}<th>{t("rc.maxRel")}</th></tr></thead>
              <tbody>{shown.map((r) => (
                <tr key={r.k}>
                  <td className="break-all font-mono">{r.k}</td>
                  {r.vals.map((v, i) => <td key={i} className="whitespace-nowrap font-mono tabular-nums">{fmtNum(v)}{i ? <Ratio v={v} base={r.vals[0]} /> : null}</td>)}
                  <td className="whitespace-nowrap font-mono tabular-nums">{!Number.isFinite(r.rel) ? t("rc.oneSide") : r.rel === Number.MAX_VALUE ? t("rc.zeroBase") : (r.rel * 100).toFixed(r.rel < 0.01 ? 4 : 2) + "%"}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        </>
      );
    }
  }
  return (
    <div className="flex flex-col gap-4 p-3">
      <div>
        <div className="mb-2 text-sm font-semibold">{t("rc.key")}</div>
        {!labels.length ? <div className="text-xs text-muted-foreground">{t("rc.noKey")}</div> : (
          <div className="overflow-x-auto"><table className="exp-table exp-table-sm"><thead>{head}</thead><tbody>
            {labels.map((l) => {
              const vals = sv.map((list) => list.find((x) => x.label === l)?.ns);
              return (
                <tr key={l}><td>{l}</td>{vals.map((v, i) => (
                  <td key={i} className="whitespace-nowrap font-mono tabular-nums">
                    {v == null ? "—" : <>{fmtSim(v)} <span className="text-muted-foreground">({Math.round(v).toLocaleString("en-US")} ns)</span></>}
                    {i ? <Ratio v={v} base={vals[0]} /> : null}
                  </td>))}</tr>
              );
            })}
          </tbody></table></div>
        )}
      </div>
      <div>
        <div className="mb-2 text-sm font-semibold">{t("rc.config")}</div>
        {!diffNames.length ? <Banner ok>{t("rc.configSame", { n: names.length })}</Banner> : (
          <>
            <div className="overflow-x-auto"><table className="exp-table exp-table-sm"><thead>{head}</thead><tbody>
              {diffNames.map((n) => (
                <tr key={n}><td className="font-mono">-{n}</td>{fs.map((f, i) => (
                  <td key={i} className="break-all bg-amber-50 font-mono dark:bg-amber-950/40">{f[n] === undefined ? DEFAULT : f[n]}</td>))}</tr>
              ))}
            </tbody></table></div>
            <div className="mt-1 text-xs text-muted-foreground">{t("rc.defaultHint")}</div>
          </>
        )}
      </div>
      <div>
        <div className="mb-2 text-sm font-semibold">{t("rc.stats")}</div>
        {stats}
      </div>
    </div>
  );
}

/** The old page's 2–4 result comparison; A is the reference, ratio = column / A. */
export default function ResultCompare({ cmp, onRemove }: { cmp: string[]; onRemove: (k: string) => void }) {
  const cache = useCache(runCache);
  const [copied, setCopied] = useState(false);
  useEffect(() => { for (const k of cmp) if (!cache[k]) void runCache.load(k); }, [cmp, cache]);
  const ds = cmp.map((k) => cache[k]);
  let body: React.ReactNode;
  if (cmp.length < 2) body = <div className="p-3 text-xs text-muted-foreground">{t("rc.needTwo")}</div>;
  else if (ds.some((d) => !d || (d.loading && !d.data))) body = <div className="p-3 text-xs text-muted-foreground">{t("rc.reading")}</div>;
  else if (ds.some((d) => d.error && !d.data)) {
    body = <div className="p-3 text-xs text-red-600">{ds.map((d, i) => (d.error ? `${L[i]}${t("common.colon")}${d.error} ` : "")).join("")}</div>;
  } else body = <Body ds={ds.map((d) => d.data as RunDetail)} />;
  return (
    <section className="overflow-hidden rounded border bg-card">
      <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
        <div className="min-w-0 flex-1"><SectionHeader title={t("rc.title")} subtitle={t("rc.sub", { n: cmp.length })} /></div>
        <span className="flex gap-2">
          <button type="button" className="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs hover:bg-muted"
            onClick={() => { void navigator.clipboard?.writeText(`${location.origin}/results?cmp=${encodeURIComponent(cmp.join(","))}`).then(() => { setCopied(true); window.setTimeout(() => setCopied(false), 1500); }); }}>
            <Link2 className="h-3.5 w-3.5" />{copied ? t("charts.linkCopied") : t("charts.copyLink")}
          </button>
          <button type="button" className="rounded border px-2 py-0.5 text-xs hover:bg-muted" onClick={() => setPrefs({ cmp: [] })}>{t("charts.clear")}</button>
        </span>
      </div>
      <div className="flex flex-wrap gap-1.5 px-3 pt-2">
        {cmp.map((k, i) => (
          <span key={k} className="inline-flex max-w-full items-center gap-1.5 rounded border bg-muted/40 px-2 py-0.5 text-xs">
            <b className="text-primary">{L[i]}</b><span className="break-all font-mono">{keyParts(k).name}</span>
            <span className="text-muted-foreground">{keyParts(k).host}</span>
            <button type="button" title={t("charts.remove")} className="text-muted-foreground hover:text-foreground" onClick={() => onRemove(k)}><X className="h-3 w-3" /></button>
          </span>
        ))}
      </div>
      {body}
    </section>
  );
}
