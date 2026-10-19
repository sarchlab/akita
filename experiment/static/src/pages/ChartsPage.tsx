import { Link2, RotateCw, Trash2, X } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import CampaignPicker from "../components/charts/CampaignPicker";
import ChartView from "../components/charts/ChartView";
import {
  baseSourceKey, buildCharts, hasBaseline, kindOf, letterOf, PLOT_LETTERS, prepareCampaign, selectionSpecs, specLabel, withSpec,
  type Campaign, type ChartCtx, type PlotPrefs,
} from "../lib/charts/model";
import { t } from "../lib/i18n";
import { THEMES } from "../lib/charts/svg";
import { campaignCache, cellsFinished, maybeLoadCampaigns, useCache } from "../lib/fetchCache";
import { MAX_PLOT, setPlot, togglePlot, usePrefs } from "../lib/prefs";
import { useStatusState } from "../lib/status";
import { useDark } from "../lib/useDark";
import { compareBase, compareValues } from "../lib/naming";
import { EmptyState, SectionHeader } from "../components/Section";
import { keyParts, runKey, type PlotSpec } from "../lib/types";

const KINDS = ["ablation", "sensitivity", "compare"] as const;

function Opt({ label, value, options, onChange, title }: {
  label: string; value: string | number; options: [string | number, string][]; onChange: (v: string) => void; title?: string;
}) {
  return (
    <label className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground" title={title}>
      {label}
      <select className="exp-select h-7 max-w-56 text-xs" value={String(value)} onChange={(e) => onChange(e.target.value)}>
        {options.map(([v, t]) => <option key={String(v)} value={String(v)}>{t}</option>)}
      </select>
    </label>
  );
}

/** Width of an element, rounded to 10 px (the old page's chartWidth). */
function useWidth<T extends HTMLElement>(): [React.RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [w, setW] = useState(960);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const measure = () => setW(Math.max(260, Math.round((el.clientWidth - 58) / 10) * 10));
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

export default function ChartsPage() {
  const { status } = useStatusState();
  const prefs = usePrefs();
  const plot = prefs.plot;
  const cache = useCache(campaignCache);
  const dark = useDark();
  const [ref, width] = useWidth<HTMLDivElement>();
  const [copied, setCopied] = useState(false);
  const sel = plot.sel;

  useEffect(() => { maybeLoadCampaigns(sel, status?.runs ?? []); }, [sel, status, cache]);

  const data = useMemo(() => (k: string): Campaign | null => {
    const d = cache[k]?.data;
    return d ? prepareCampaign(d, k) : null;
  }, [cache]);
  const { specs, derived } = useMemo(
    () => selectionSpecs(sel, data, (p) => status?.projects?.[p]?.plots as PlotSpec[] | null | undefined),
    [sel, data, status]);
  const ctx: ChartCtx = useMemo(() => ({
    plot, data, specs,
    statusKind: (k) => status?.runs.find((r) => runKey(r) === k)?.kind ?? null,
  }), [plot, data, specs, status]);
  const built = useMemo(() => {
    const b = buildCharts(ctx);
    // Campaigns that differ by their compare_by value (e.g. config wide / base)
    // take that value's colour, the same as its chip on the other pages.
    if (status && b.kind === "compare") {
      const values = compareValues(status.runs, status.projects);
      for (const m of b.models) {
        const bases = m.series.map((s) => {
          const r = status.runs.find((x) => runKey(x) === s.src);
          return r ? compareBase(r, status.projects) : null;
        });
        if (bases.every((x) => x != null) && new Set(bases).size === bases.length) {
          m.series.forEach((s, i) => { s.colorIdx = values.indexOf(bases[i] as string); });
        }
      }
    }
    return b;
  }, [ctx, status]);
  const th = THEMES[dark ? "dark" : "light"];
  const set = (patch: Partial<PlotPrefs>) => setPlot(patch);
  const spec = built.spec;
  const sctx = spec ? withSpec(ctx, spec) : null;

  const toggle = (k: string) => { if (!togglePlot(k)) window.alert(t("charts.full", { max: MAX_PLOT })); };

  const chips = sel.map((key, i) => {
    const p = keyParts(key), st = cache[key], raw = st?.data, d = data(key);
    const run = status?.runs.find((r) => runKey(r) === key);
    let info = st?.loading && !raw ? t("charts.loading") : st?.error && !raw ? t("charts.loadFailed") : "";
    if (d && raw) info = `${d.counts.ok || 0}/${d.total}` + (cellsFinished(raw) ? "" : t("charts.stillRunning")) + (st?.loading ? t("charts.updating") : "");
    else if (run?.progress && !info) info = `${run.progress.finished}/${run.progress.total ?? "?"}`;
    let baseSel = null;
    if (d && sctx && !d.sweep && sel.length > 1) {
      const resolved = baseSourceKey(sctx, key);
      const cands = sel.filter((k) => k === resolved || hasBaseline(sctx, k));
      if (cands.length > 1 || resolved !== key) {
        baseSel = (
          <label className="inline-flex items-center gap-1" title={t("charts.baseHint")}>
            baseline
            <select className="exp-select h-6 px-1 text-xs" value={resolved === key ? "" : resolved}
              onChange={(e) => set({ base: { ...plot.base, [key]: e.target.value } })}>
              {cands.map((k) => <option key={k} value={k === key ? "" : k}>{k === key ? t("charts.self") : PLOT_LETTERS[sel.indexOf(k)]}</option>)}
            </select>
          </label>
        );
      }
    }
    return (
      <span key={key} className="inline-flex max-w-full flex-wrap items-center gap-x-2 gap-y-1 rounded border bg-muted/40 px-2 py-1 text-xs">
        <b className="text-primary">{PLOT_LETTERS[i]}</b>
        <span className="break-all font-mono">{p.name}</span>
        <span className="text-muted-foreground">{p.host}</span>
        {kindOf(ctx, key) === "sensitivity" ? <span className="rounded border px-1 text-[11px] text-muted-foreground">{t("common.sensitivity")}</span> : null}
        <span className="font-mono text-muted-foreground">{info}</span>
        {baseSel}
        <button type="button" title={t("charts.remove")} className="text-muted-foreground hover:text-foreground" onClick={() => toggle(key)}><X className="h-3.5 w-3.5" /></button>
      </span>
    );
  });

  const seriesName = (m: string) => {
    if (!spec) return m;
    for (const k of sel) { const d = data(k); if (d?.doc.labels?.[spec.series]?.[m]) return d.label(spec.series, m); }
    return m;
  };
  const options: React.ReactNode[] = [];
  if (built.specs.length > 1 && spec) {
    options.push(<Opt key="spec" label={t("charts.opt.plot")} value={spec.name} onChange={(v) => set({ spec: v })}
      options={built.specs.map((s) => [s.name, specLabel(s) === s.name ? s.name : `${specLabel(s)} (${s.name})`])} />);
  }
  const sens = built.sens;
  if (built.kind === "sensitivity" && sens) {
    if (sens.skeys.length > 1) options.push(<Opt key="sens" label="campaign" value={sens.skey} onChange={(v) => set({ sens: v })}
      options={sens.skeys.map((k) => [k, `${letterOf(ctx, k)} ${keyParts(k).name}`])} />);
    const refOpts: [string, string][] = [];
    if (sens.d.sens?.reference) refOpts.push(["own", t("charts.opt.refOwn", { name: sens.d.sens.refName })]);
    else for (const k of sens.mains) refOpts.push([k, `${letterOf(ctx, k)} ${keyParts(k).name}`]);
    refOpts.push(["none", t("charts.opt.refNone")]);
    options.push(<Opt key="ref" label={t("charts.opt.ref")} value={sens.ref} options={refOpts} onChange={(v) => set({ ref: v })}
      title={t("charts.opt.refHint")} />);
    options.push(<Opt key="norm" label={t("charts.opt.norm")} value={sens.norm} onChange={(v) => set({ norm: v })}
      options={[["matched", t("charts.opt.normMatched")], ["fixed", t("charts.opt.normFixed")]]} />);
    if (sens.norm === "fixed") {
      options.push(<Opt key="fixed" label={t("charts.opt.fixed")} value={sens.fixedKey ?? ""} onChange={(v) => set({ fixed: v })}
        title={t("charts.opt.fixedHint")}
        options={sens.fixedKeys.map((k) => [k, k === sens.skey ? t("charts.opt.fixedOwn", { name: sens.d.sens?.refName })
          : t("charts.opt.fixedOther", { letter: letterOf(ctx, k), name: keyParts(k).name })])} />);
    }
    options.push(<Opt key="config" label={spec?.series || t("charts.opt.series")} value={sens.config} onChange={(v) => set({ config: v })}
      options={sens.modes.map((m) => [m, seriesName(m)])} />);
  } else if (built.kind === "compare" && built.modes) {
    options.push(<Opt key="config" label={spec?.series || t("charts.opt.series")} value={built.config ?? ""} onChange={(v) => set({ config: v })}
      options={built.modes.map((m) => [m, seriesName(m)])} />);
  }
  options.push(<Opt key="cap" label={t("charts.opt.cap")} value={plot.cap || 0} onChange={(v) => set({ cap: Number(v) })}
    title={t("charts.opt.capHint")} options={[[0, t("charts.opt.capNone")], [2, "2×"], [3, "3×"], [5, "5×"]]} />);

  const msgs: React.ReactNode[] = [];
  for (const k of sel) {
    const st = cache[k];
    if (!st?.data && (!st || st.loading)) msgs.push(<div key={"l" + k} className="text-muted-foreground">{t("charts.reading", { letter: letterOf(ctx, k), name: keyParts(k).name })}</div>);
    else if (st?.error) msgs.push(
      <div key={"e" + k} className="text-red-600 dark:text-red-400">{t("charts.readFailed", { letter: letterOf(ctx, k), error: st.error })}
        <button type="button" className="ml-2 inline-flex items-center gap-1 rounded border px-1.5 text-xs text-foreground hover:bg-muted" onClick={() => void campaignCache.load(k, true, st.seen ?? null)}>
          <RotateCw className="h-3 w-3" />{t("charts.retry")}</button></div>);
  }
  if (derived && spec) msgs.push(<div key="derived" className="text-muted-foreground">{t("charts.noPlots", { label: `${specLabel(spec)}: ${spec.x} × ${spec.series}` })}</div>);
  built.notes.forEach((n, i) => msgs.push(<div key={"n" + i} className="text-muted-foreground">{n}</div>));
  (built.warns ?? []).forEach((n, i) => msgs.push(<div key={"w" + i} className="break-words text-amber-700 dark:text-amber-300">{n}</div>));

  const explain = spec ? t(spec.lower_is_better ? "charts.explain" : "charts.explainHigher", {
    label: specLabel(spec), baseline: spec.baseline ?? "baseline", metric: spec.metric, x: spec.x,
    lower: t("charts.lower"), higher: t("charts.higher"),
    match: spec.match?.length ? t("charts.explainMatch", { match: spec.match.join(", ") }) : "",
  }) : "";

  return (
    <div className="exp-page h-full overflow-auto md:flex md:overflow-hidden">
      <aside className="border-b bg-card md:w-72 md:shrink-0 md:overflow-hidden md:border-b-0 md:border-r">
        {status ? <CampaignPicker status={status} sel={sel} showArchived={prefs.showArchived} onToggle={toggle} />
          : <div className="p-6 text-sm text-muted-foreground">{t("common.connecting")}</div>}
      </aside>
      <div ref={ref} className="min-w-0 p-3 md:flex-1 md:overflow-auto">
        <section className="rounded border bg-card">
          <div className="flex flex-wrap items-center gap-2 border-b px-4 py-3">
            <div className="min-w-0 flex-1"><SectionHeader title={t("charts.title")} subtitle={t("charts.selected", { n: sel.length, max: MAX_PLOT })} /></div>
            <span className="flex gap-2">
              <button type="button" disabled={!sel.length} className="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs hover:bg-muted disabled:opacity-50"
                onClick={() => { void navigator.clipboard?.writeText(`${location.origin}/charts?plot=${encodeURIComponent(sel.join(","))}`).then(() => { setCopied(true); window.setTimeout(() => setCopied(false), 1500); }); }}>
                <Link2 className="h-3.5 w-3.5" />{copied ? t("charts.linkCopied") : t("charts.copyLink")}
              </button>
              <button type="button" disabled={!sel.length} className="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs hover:bg-muted disabled:opacity-50"
                onClick={() => set({ sel: [], base: {}, hide: [] })}>
                <Trash2 className="h-3.5 w-3.5" />{t("charts.clear")}
              </button>
            </span>
          </div>
          {explain ? <div className="px-4 py-2 text-xs text-muted-foreground">{explain}</div> : null}
          {sel.length ? <>
            <div className="flex flex-wrap gap-1.5 px-4 pt-2">{chips}</div>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
              <div className="inline-flex overflow-hidden rounded-md border">
                {KINDS.map((k) => (
                  <button key={k} type="button" title={t(`charts.kind.${k}.hint`)} disabled={!built.avail[k]} onClick={() => set({ kind: k })}
                    className={`border-l px-3 py-1 text-xs font-medium first:border-l-0 disabled:opacity-40 ${built.kind === k ? "bg-primary text-primary-foreground" : "bg-card hover:bg-muted"}`}>
                    {t(`charts.kind.${k}`)}
                  </button>
                ))}
              </div>
              {options}
            </div>
            {msgs.length ? <div className="flex flex-col gap-1 px-4 pb-3 text-xs">{msgs}</div> : null}
          </> : (
            <EmptyState title={t("charts.none")} hint={t("charts.noneHint")} />
          )}
        </section>
        {built.models.length ? (
          <section className="mt-3 rounded border bg-card">
            {built.models.map((m) => (
              <ChartView key={m.id} m={m} th={th} width={width} hide={plot.hide}
                onToggleHide={(h) => set({ hide: plot.hide.includes(h) ? plot.hide.filter((x) => x !== h) : [...plot.hide, h] })} />
            ))}
          </section>
        ) : null}
      </div>
    </div>
  );
}
