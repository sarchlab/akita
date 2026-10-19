// Chart models for campaigns: generic over CampaignCells (herd-cells/1) and the
// project's plot specs (herd.json "plots"). A port of the original monitor's
// chart code (pairOf, baseSourceKey, finishModel, ablationModels,
// compareModel, sensitivitySetup/sensitivityModels, chartCSV) with every
// simulator-specific name (metrics, baseline, series values, labels) taken
// from the spec and the cells.
//
// value = baseline metric / cell metric (lower_is_better) or the inverse, for
// the same `x` value, only when every `match` metric is equal; GMEAN =
// geometric mean over the `x` values that every plotted series has.
import { getLocale, t } from "../i18n.ts";
import { keyParts, type CampaignCells, type CellDoc, type PlotSpec } from "../types.ts";

export const PLOT_LETTERS = "ABCD";

// ---------- flags ----------

const FLAG_GLYPH: Record<string, string> = {
  missing: "–", running: "…", failed: "×", bad: "!", "no-baseline": "∅", mismatch: "≠",
};
const FLAG_CRIT = new Set(["failed", "bad", "no-baseline", "mismatch"]);
export const FLAG_ORDER = ["mismatch", "failed", "bad", "no-baseline", "running", "missing"];

/** Glyph, text and severity of a cell flag (match: the spec's matched metrics, for "mismatch"). */
export function flagInfo(flag: string, match?: string[]): { glyph: string; text: string; crit: boolean } | null {
  if (!(flag in FLAG_GLYPH)) return null;
  const text = flag === "mismatch" ? t("flag.mismatch", { match: (match ?? []).join(", ") })
    : t(`flag.${flag}` as "flag.missing");
  return { glyph: FLAG_GLYPH[flag], text, crit: FLAG_CRIT.has(flag) };
}

// ---------- preferences ----------

export interface PlotPrefs {
  sel: string[];
  kind: string;
  /** Name of the chosen plot spec ("" = the first that fits the chart kind). */
  spec: string;
  base: Record<string, string>;
  /** The series value shown in compare / sensitivity charts ("" = automatic). */
  config: string;
  ref: string;
  norm: string;
  fixed: string;
  cap: number;
  hide: string[];
  sens: string;
}

export function defaultPlot(): PlotPrefs {
  return { sel: [], kind: "", spec: "", base: {}, config: "", ref: "auto", norm: "matched", fixed: "auto", cap: 0, hide: [], sens: "" };
}

// ---------- campaigns ----------

export type Rest = Record<string, string>;

export interface SensPoint { name: string; cell: Rest; value: number | null; tick: string; legend: string }
export interface SensDim {
  name: string; title: string; xlabel: string;
  ref_value: number | null; ref_tick: string; ref_legend: string;
  points: SensPoint[];
}
export interface SensLayout {
  /** Key values of the campaign's own reference point, if it ran one. */
  reference: Rest | null;
  refName: string | null;
  dims: SensDim[];
}

/** A loaded campaign, indexed for the chart code. */
export interface Campaign {
  key: string;
  name: string;
  host: string;
  project: string;
  doc: CampaignCells;
  dims: string[];
  sweep: boolean;
  cells: Map<string, CellDoc>;
  counts: Record<string, number>;
  total: number;
  finished: boolean;
  warnings: string[];
  binary: string;
  meta: Record<string, unknown>;
  sens: SensLayout | null;
  /** Values of a dimension: the declared order, then others in order of appearance. */
  order(dim: string): string[];
  /** The values of a dimension that some cell has, in order. */
  present(dim: string): string[];
  label(dim: string, value: string): string;
}

const SEP = "\u001f";

function cellId(dims: string[], key: Rest): string {
  return dims.map((d) => key[d] ?? "").join(SEP);
}

const prepared = new WeakMap<CampaignCells, Map<string, Campaign>>();

/** The campaign of a CampaignCells document ("host|project|name" key); memoized per document. */
export function prepareCampaign(doc: CampaignCells, key: string): Campaign {
  let byKey = prepared.get(doc);
  if (!byKey) { byKey = new Map(); prepared.set(doc, byKey); }
  const hit = byKey.get(key);
  if (hit) return hit;
  const { host, project, name } = keyParts(key);
  const dims = doc.dimensions?.length ? doc.dimensions : [...new Set((doc.cells ?? []).flatMap((c) => Object.keys(c.key ?? {})))];
  const cells = new Map<string, CellDoc>();
  for (const c of doc.cells ?? []) cells.set(cellId(dims, c.key ?? {}), c);
  const counts: Record<string, number> = { ok: 0, failed: 0, running: 0, missing: 0, bad: 0 };
  for (const c of cells.values()) counts[c.status] = (counts[c.status] || 0) + 1;
  const orders = new Map<string, string[]>(), presents = new Map<string, string[]>();
  const order = (dim: string) => {
    let o = orders.get(dim);
    if (!o) {
      o = [...(doc.order?.[dim] ?? [])];
      for (const c of cells.values()) {
        const v = c.key?.[dim] ?? "";
        if (!o.includes(v)) o.push(v);
      }
      orders.set(dim, o);
    }
    return o;
  };
  const present = (dim: string) => {
    let p = presents.get(dim);
    if (!p) {
      const has = new Set([...cells.values()].map((c) => c.key?.[dim] ?? ""));
      p = order(dim).filter((v) => has.has(v));
      presents.set(dim, p);
    }
    return p;
  };
  const c: Campaign = {
    key, name: doc.name || name, host: doc.host || host, project: doc.project || project, doc, dims,
    sweep: !!doc.sweep?.dimensions?.length,
    cells, counts, total: doc.total ?? cells.size,
    finished: (counts.running || 0) + (counts.missing || 0) === 0,
    warnings: doc.warnings ?? [], binary: doc.binary ?? "", meta: doc.meta ?? {},
    sens: null, order, present,
    label: (dim, v) => doc.labels?.[dim]?.[v] ?? v,
  };
  c.sens = c.sweep ? sensLayout(doc) : null;
  byKey.set(key, c);
  return c;
}

const restName = (r: Rest | null) => (r ? Object.values(r).join(",") : "");

function sensLayout(doc: CampaignCells): SensLayout {
  let reference: Rest | null = null;
  const dims: SensDim[] = [];
  for (const d of doc.sweep?.dimensions ?? []) {
    const legendOf = (p: { value: number | null; label: string }) => (p.value == null || !d.unit ? p.label : `${p.label} ${d.unit}`);
    const ref = d.points.find((p) => p.reference);
    if (ref?.cell && !reference) reference = ref.cell;
    const refValue = ref ? ref.value : null;
    const refTick = ref && ref.value != null && ref.label ? ref.label : t("sens.reference");
    dims.push({
      name: d.name, title: d.label || d.name, xlabel: d.unit ? `${d.label || d.name} (${d.unit})` : d.label || d.name,
      ref_value: refValue, ref_tick: refTick,
      ref_legend: ref && ref.value != null && ref.label ? legendOf(ref) : refTick,
      points: d.points.filter((p) => !p.reference && p.cell).map((p) => ({
        name: restName(p.cell), cell: p.cell as Rest, value: p.value, tick: p.label, legend: legendOf(p),
      })),
    });
  }
  return { reference, refName: reference ? restName(reference) : null, dims };
}

// ---------- spec helpers ----------

/** A plot spec for a campaign whose project has none: the first metric, first dimension against the last. */
export function defaultSpecs(c: Campaign | null): PlotSpec[] {
  if (!c || !c.dims.length) return [];
  const metric = [...c.cells.values()].map((x) => Object.keys(x.metrics ?? {})[0]).find(Boolean);
  if (!metric) return [];
  const x = c.dims[0], series = c.dims[c.dims.length - 1];
  const spec: PlotSpec = { name: "default", label: metric, kind: c.sweep ? "sweep" : "ratio", metric, x, series,
    baseline: c.present(series)[0], match: [], summary: "gmean" };
  return [spec];
}

/** The specs that draw a chart kind: sweep specs for sensitivity (else ratio ones), ratio specs otherwise. */
export function specsFor(specs: PlotSpec[], kind: string): PlotSpec[] {
  const ratio = specs.filter((s) => s.kind !== "sweep");
  if (kind !== "sensitivity") return ratio;
  const sweep = specs.filter((s) => s.kind === "sweep");
  return sweep.length ? sweep : ratio;
}

export function specLabel(spec: PlotSpec): string {
  return spec.label || spec.name;
}

// ---------- context ----------

export interface ChartCtx {
  plot: PlotPrefs;
  /** Loaded campaign for a "host|project|name" key. */
  data(key: string): Campaign | null;
  /** Plot specs of the selection (the project's herd.json "plots"). */
  specs: PlotSpec[];
  /** The status list's idea of the kind ("sensitivity" / "campaign"), before data loads. */
  statusKind?(key: string): string | null;
  /** Set by buildCharts: the spec being drawn. */
  spec?: PlotSpec;
}

export interface Pair {
  flag: string;
  note?: string;
  s?: number;
  cell: CellDoc | null;
  base: CellDoc | null;
}

export interface Series {
  key: string;
  hideKey: string;
  label: string;
  long?: string;
  tick?: string;
  pal: "cat" | "camp" | "seq";
  idx: number;
  n?: number;
  src: string;
  baseSrc: string;
  variant: string;
  mode: string;
  pointValue?: number | null;
  ref?: boolean;
  pairs: Record<string, Pair>;
  gmean: number | null;
  /** Display only: an index into the theme's categorical palette that overrides the series colour. */
  colorIdx?: number;
}

export interface LinePoint { tick: string; legend: string; v: number | null; hidden: boolean; ref?: boolean }
export interface LineModel {
  type: "line"; id: string; title: string; xTitle: string; xName: string; yLabel: string;
  points: LinePoint[]; n: number; total: number;
}

export interface BarModel {
  type: "bars";
  id: string;
  title: string;
  xTitle?: string;
  spec: PlotSpec;
  workloads: string[];
  labels: Record<string, string>;
  series: Series[];
  legend: boolean;
  noun: string;
  dim?: string;
  summary: boolean;
  visible: Series[];
  empty: Series[];
  counted: Series[];
  common: string[];
  flagCounts: Record<string, number>;
  cap: number;
  sub: string[];
  warn: string[];
  diffs?: string[];
  line?: LineModel;
}

export type ChartModel = BarModel | LineModel;

export interface ChartResult {
  models: BarModel[];
  notes: string[];
  warns?: string[];
  modes?: string[];
  config?: string;
}

// ---------- data helpers ----------

export function letterOf(ctx: ChartCtx, key: string): string {
  const i = ctx.plot.sel.indexOf(key);
  return i >= 0 ? PLOT_LETTERS[i] : "?";
}

export function kindOf(ctx: ChartCtx, key: string): string {
  const d = ctx.data(key);
  if (d) return d.sweep ? "sensitivity" : "main";
  return ctx.statusKind?.(key) === "sensitivity" ? "sensitivity" : "main";
}

function specOf(ctx: ChartCtx): PlotSpec {
  if (!ctx.spec) throw new Error("no plot spec");
  return ctx.spec;
}

/** Values of the dimensions other than x and series that select a campaign's plain cells. */
export function restOf(c: Campaign, spec: PlotSpec): Rest {
  const rest: Rest = {};
  for (const d of c.dims) {
    if (d === spec.x || d === spec.series) continue;
    const vals = c.present(d);
    rest[d] = vals.includes("") ? "" : vals[0] ?? "";
  }
  return rest;
}

/** The plain configuration of a campaign: none for a sweep (its cells are points). */
function mainRest(c: Campaign, spec: PlotSpec): Rest | null {
  return c.sweep ? null : restOf(c, spec);
}

function cellAt(spec: PlotSpec, c: Campaign | null, x: string, rest: Rest | null, mode: string): CellDoc | null {
  if (!c || !rest) return null;
  return c.cells.get(cellId(c.dims, { ...restOf(c, spec), ...rest, [spec.x]: x, [spec.series]: mode })) || null;
}

/** Whether a campaign has baseline cells (any status) for `rest` (null: its plain configuration). */
export function hasBaseline(ctx: ChartCtx, key: string, rest: Rest | null = null): boolean {
  const c = ctx.data(key), spec = specOf(ctx);
  if (!c) return false;
  const r = rest ?? mainRest(c, spec);
  if (!r) return false;
  const want = { ...restOf(c, spec), ...r };
  for (const cell of c.cells.values()) {
    if ((cell.key[spec.series] ?? "") !== spec.baseline) continue;
    if (Object.entries(want).every(([d, v]) => (cell.key[d] ?? "") === v)) return true;
  }
  return false;
}

/** Series values (minus the baseline unless asked) of campaigns, in their declared order. */
export function plotModes(ctx: ChartCtx, keys: string[], withBaseline: boolean): string[] {
  const spec = specOf(ctx);
  const out: string[] = [];
  let hasBase = false;
  for (const k of keys) {
    for (const m of ctx.data(k)?.present(spec.series) ?? []) {
      if (m === spec.baseline) { hasBase = true; continue; }
      if (!out.includes(m)) out.push(m);
    }
  }
  return withBaseline && hasBase ? [spec.baseline as string, ...out] : out;
}

/**
 * The chosen series value, else the one most of the campaigns have (the last
 * in order among those: the "full" configuration usually comes last).
 */
function pickConfig(ctx: ChartCtx, modes: string[], keys: string[]): string {
  if (modes.includes(ctx.plot.config)) return ctx.plot.config;
  const spec = specOf(ctx);
  const n = (m: string) => keys.filter((k) => ctx.data(k)?.present(spec.series).includes(m)).length;
  const most = Math.max(...modes.map(n));
  return modes.filter((m) => n(m) === most).pop() as string;
}

function seriesLabel(ctx: ChartCtx, keys: string[], mode: string): string {
  const spec = specOf(ctx);
  for (const k of keys) {
    const c = ctx.data(k);
    if (c && c.doc.labels?.[spec.series]?.[mode]) return c.label(spec.series, mode);
  }
  return mode;
}

// ---------- values ----------

export function geomean(vals: number[]): number | null {
  return vals.length ? Math.exp(vals.reduce((a, v) => a + Math.log(v), 0) / vals.length) : null;
}

/** Display of a metric value: seconds-like metrics as time, others as numbers. */
export function fmtMetric(metric: string, v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "—";
  if (/time/i.test(metric)) {
    const ns = v * 1e9, a = Math.abs(ns);
    return a >= 1e9 ? (ns / 1e9).toFixed(3) + " s" : a >= 1e6 ? (ns / 1e6).toFixed(2) + " ms"
      : a >= 1e3 ? (ns / 1e3).toFixed(1) + " µs" : ns.toFixed(0) + " ns";
  }
  return Number.isInteger(v) ? String(v) : v.toPrecision(6).replace(/\.?0+$/, "");
}

export function pairOf(spec: PlotSpec, cell: CellDoc | null, base: CellDoc | null): Pair {
  if (!cell) return { flag: "missing", note: t("pair.noCell"), cell, base };
  if (cell.status !== "ok") return { flag: cell.status, note: cell.reason, cell, base };
  const cv = cell.metrics?.[spec.metric];
  if (cv == null || !(cv > 0)) return { flag: "bad", note: t("pair.noMetric", { metric: spec.metric }), cell, base };
  const bv = base?.metrics?.[spec.metric];
  if (!base || base.status !== "ok" || bv == null || !(bv > 0)) {
    const what = !base ? t("pair.baselineMissing")
      : base.status !== "ok" ? `${flagInfo(base.status, spec.match)?.text || base.status}${base.reason ? t("common.sep") + base.reason : ""}`
      : t("pair.noMetric", { metric: spec.metric });
    return { flag: "no-baseline", note: t("pair.baseline", { what }), cell, base };
  }
  for (const m of spec.match ?? []) {
    const a = base.metrics?.[m], b = cell.metrics?.[m];
    if (a !== b) return { flag: "mismatch", note: t("pair.mismatch", { metric: m, base: a ?? "—", cell: b ?? "—" }), cell, base };
  }
  return { flag: "ok", s: spec.lower_is_better ? bv / cv : cv / bv, cell, base };
}

/**
 * Whose baseline cells divide this campaign: the chosen one, else its own, else
 * the first other selected campaign with baselines (same host first).
 */
export function baseSourceKey(ctx: ChartCtx, key: string): string {
  const want = ctx.plot.base[key];
  if (want && want !== key && ctx.plot.sel.includes(want) && hasBaseline(ctx, want)) return want;
  if (hasBaseline(ctx, key)) return key;
  const host = keyParts(key).host;
  const others = ctx.plot.sel.filter((k) => k !== key && hasBaseline(ctx, k));
  return others.find((k) => keyParts(k).host === host) || others[0] || key;
}

/** Name parts that differ between the campaigns ("wide" vs "fast"). */
export function campShortNames(keys: string[]): Record<string, string> {
  const toks = keys.map((k) => keyParts(k).name.split("_"));
  const common = toks.length > 1 ? toks[0].filter((tk) => toks.every((ts) => ts.includes(tk))) : [];
  const out: Record<string, string> = {};
  keys.forEach((k, i) => { out[k] = toks[i].filter((tk) => !common.includes(tk)).join("_") || keyParts(k).name; });
  const vals = Object.values(out);
  for (const k of keys) if (vals.filter((v) => v === out[k]).length > 1) out[k] += "@" + keyParts(k).host;
  return out;
}

function mergeOrder(lists: string[][]): string[] {
  const out: string[] = [];
  for (const l of lists) for (const x of l) if (!out.includes(x)) out.push(x);
  return out;
}

function labelsOf(c: Campaign, dim: string): Record<string, string> {
  return Object.fromEntries(c.order(dim).map((v) => [v, c.label(dim, v)]));
}

type Draft = Pick<BarModel, "type" | "id" | "title" | "workloads" | "labels" | "series" | "legend" | "noun"> &
  Partial<Pick<BarModel, "xTitle" | "dim">>;

export function finishModel(ctx: ChartCtx, d: Draft): BarModel {
  const m = d as BarModel;
  m.spec = specOf(ctx);
  m.summary = m.spec.summary === "gmean";
  // A chart with one series draws no legend, so a remembered hide could not be undone there.
  m.visible = m.series.length < 2 ? m.series.slice() : m.series.filter((s) => !ctx.plot.hide.includes(s.hideKey));
  // GMEAN over the x values that every visible series with any result has; a
  // series without a single result yet is left out instead of emptying
  // everyone's GMEAN.
  m.empty = m.visible.filter((s) => !m.workloads.some((w) => s.pairs[w] && s.pairs[w].flag === "ok"));
  m.counted = m.visible.filter((s) => !m.empty.includes(s));
  m.common = m.counted.length ? m.workloads.filter((w) => m.counted.every((s) => s.pairs[w] && s.pairs[w].flag === "ok")) : [];
  for (const s of m.series) s.gmean = null;
  if (m.summary) for (const s of m.counted) s.gmean = m.common.length ? geomean(m.common.map((w) => s.pairs[w].s as number)) : null;
  m.flagCounts = {};
  for (const s of m.visible) for (const w of m.workloads) {
    const f = s.pairs[w].flag;
    if (f !== "ok") m.flagCounts[f] = (m.flagCounts[f] || 0) + 1;
  }
  m.cap = +ctx.plot.cap || 0;
  m.sub = [];
  m.warn = [];
  return m;
}

export function nounOf(m: BarModel): string {
  const n = m.noun || t("noun.series");
  if (getLocale() !== "zh") return n;
  return /^[\x00-\x7f]/.test(n) ? ` ${n} ` : n;
}

export function gmeanText(m: BarModel): string {
  const noun = nounOf(m), x = m.spec.x, L = t("common.list");
  if (!m.visible.length) return t("gmean.allHidden");
  if (!m.summary) return "";
  const empty = m.empty.length ? t("gmean.empty", { names: m.empty.map((s) => s.label).join(L) }) : "";
  if (!m.counted.length) return t("gmean.none");
  if (!m.common.length) return t("gmean.noCommon", { n: m.counted.length, noun, x }) + empty;
  const missing = m.workloads.filter((w) => !m.common.includes(w)).map((w) => m.labels[w] || w);
  const head = m.counted.length === 1 ? t("gmean.one", { k: m.common.length, total: m.workloads.length, x })
    : t("gmean.many", { n: m.counted.length, noun, k: m.common.length, total: m.workloads.length, x });
  return head + (missing.length ? t("gmean.without", { list: missing.slice(0, 8).join(L), more: missing.length > 8 ? t("gmean.more") : "" }) : "") + empty;
}

// ---------- provenance ----------
// When cells and their baseline come from different campaigns, say whether the
// two ran the same binary and flags (the commands of one x value).

interface Src { key: string; rest: Rest | null; mode: string; variant: string }

/** The x value whose commands stand for the campaign: the first in order that has any. */
function cmdX(c: Campaign, spec: PlotSpec): string | null {
  for (const x of c.order(spec.x)) {
    for (const cell of c.cells.values()) if ((cell.key[spec.x] ?? "") === x && cell.command?.length) return x;
  }
  return null;
}

function cmdOf(ctx: ChartCtx, src: Src): string[] | null {
  const c = ctx.data(src.key), spec = specOf(ctx);
  if (!c) return null;
  const x = cmdX(c, spec);
  if (x == null) return null;
  return cellAt(spec, c, x, src.rest, src.mode)?.command || null;
}

function flagList(fs: string[]): string {
  const short = fs.map((f) => (f.length > 60 ? f.slice(0, 57) + "…" : f));
  return short.length > 6 ? short.slice(0, 6).join(" ") + t("prov.flagsMore", { n: short.length }) : short.join(" ");
}

const strList = (v: unknown): string[] => (Array.isArray(v) ? v.map(String) : []);

export function provenance(ctx: ChartCtx, a: Src, b: Src, ignorePoint: boolean): { same: boolean; text: string; noMode?: boolean } | null {
  const da = ctx.data(a.key), db = ctx.data(b.key), spec = specOf(ctx);
  if (!da || !db) return null;
  const L = (k: string) => letterOf(ctx, k);
  const parts: string[] = [];
  if (da.binary && db.binary && da.binary !== db.binary) parts.push(t("prov.binary", { a: L(a.key), ba: da.binary, b: L(b.key), bb: db.binary }));
  const xa = cmdX(da, spec), xb = cmdX(db, spec);
  const ca = cmdOf(ctx, a), cb = cmdOf(ctx, b);
  const ignore = [...strList(da.meta.ignore_flags), ...strList(db.meta.ignore_flags)];
  const point = ignorePoint ? [...strList(da.meta.sweep_flags), ...strList(db.meta.sweep_flags)] : [];
  if (ca && cb && xa === xb) {
    const skip = (f: string) => ignore.some((p) => f.startsWith(p)) || point.some((p) => f.startsWith(p));
    const A = new Set(ca.filter((f) => !skip(f))), B = new Set(cb.filter((f) => !skip(f)));
    const onlyA = [...A].filter((f) => !B.has(f)), onlyB = [...B].filter((f) => !A.has(f));
    if (onlyA.length) parts.push(t("prov.only", { l: L(a.key), flags: flagList(onlyA) }));
    if (onlyB.length) parts.push(t("prov.only", { l: L(b.key), flags: flagList(onlyB) }));
  } else if (!da.binary || !db.binary) {
    return null;
  } else if (da.binary === db.binary) {
    return { same: true, noMode: true, text: t("prov.sameBinaryNoMode", { a: L(a.key), b: L(b.key), bin: da.binary, mode: a.mode }) };
  }
  const v = (s: string) => (s ? t("prov.variant", { v: s }) : "");
  const what = t("prov.what", { a: L(a.key), am: a.mode, av: v(a.variant), b: L(b.key), bm: b.mode, bv: v(b.variant) });
  const pt = ignorePoint ? t("prov.ignorePoint") : "";
  const x = xa ?? "?";
  return parts.length
    ? { same: false, text: t("prov.differ", { what, x, pt, parts: parts.join(t("prov.parts")) }) }
    : { same: true, text: t("prov.same", { what, x, pt }) };
}

export function campLine(d: Campaign): string {
  const c = d.counts;
  const parts = [t("camp.cells", { host: d.host, ok: c.ok || 0, total: d.total })];
  if (c.running) parts.push(t("camp.running", { n: c.running }));
  if (c.missing) parts.push(t("camp.missing", { n: c.missing }));
  if (c.failed) parts.push(t("camp.failed", { n: c.failed }));
  const rev = d.meta.git_revision;
  if (rev) parts.push(`git ${String(rev).slice(0, 9)}`);
  return parts.join(" · ");
}

// ---------- chart kinds ----------

export function ablationModels(ctx: ChartCtx): ChartResult {
  const spec = specOf(ctx);
  const models: BarModel[] = [], notes: string[] = [];
  const L = (k: string) => letterOf(ctx, k);
  for (const key of ctx.plot.sel) {
    const d = ctx.data(key);
    if (!d) continue;
    if (d.sweep) { notes.push(t("abl.isSweep", { l: L(key) })); continue; }
    const modes = d.present(spec.series).filter((m) => m !== spec.baseline);
    if (!modes.length) { notes.push(t("abl.onlyBaseline", { l: L(key) })); continue; }
    const bkey = baseSourceKey(ctx, key), bd = ctx.data(bkey);
    const rest = restOf(d, spec), brest = bd ? restOf(bd, spec) : null;
    const workloads = d.order(spec.x);
    const series: Series[] = modes.map((mode, i) => ({
      key: mode, hideKey: "cfg:" + mode, label: d.label(spec.series, mode),
      pal: "cat", idx: i, src: key, baseSrc: bkey, variant: "", mode, gmean: null,
      pairs: Object.fromEntries(workloads.map((w) => [w, pairOf(spec, cellAt(spec, d, w, rest, mode), cellAt(spec, bd, w, brest, spec.baseline as string))])),
    }));
    const m = finishModel(ctx, {
      type: "bars", id: "ablation_" + d.name, title: t("abl.title", { l: L(key), name: d.name }),
      workloads, labels: labelsOf(d, spec.x), series, legend: series.length > 1, noun: t("noun.config"),
    });
    m.sub = [campLine(d),
      bkey !== key ? t("abl.baseFrom", { l: L(bkey), name: keyParts(bkey).name })
        : hasBaseline(ctx, key) ? t("abl.baseOwn") : t("abl.baseNone"),
      gmeanText(m)].filter(Boolean);
    for (const n of strList(d.meta.notes)) m.sub.push(n);
    m.warn = d.warnings.slice(0, 3);
    if (bkey !== key) {
      const seen = new Set<string>();
      for (const mode of modes) {
        const pv = provenance(ctx, { key, rest, mode, variant: "" }, { key: bkey, rest: brest, mode, variant: "" }, false);
        if (!pv) continue;
        const text = pv.noMode ? t("prov.sameBinaryNoSame", { a: L(key), b: L(bkey), bin: d.binary }) : pv.text;
        if (seen.has(text)) continue;
        seen.add(text);
        (pv.same ? m.sub : m.warn).push(text);
      }
    }
    models.push(m);
  }
  return { models, notes };
}

export function compareModel(ctx: ChartCtx): ChartResult {
  const spec = specOf(ctx);
  const keys = ctx.plot.sel.filter((k) => ctx.data(k) && !ctx.data(k)!.sweep);
  if (keys.length < 2) return { models: [], notes: [t("cmp.needTwo")] };
  const modes = plotModes(ctx, keys, false);
  if (!modes.length) return { models: [], notes: [t("cmp.onlyBaseline")] };
  const config = pickConfig(ctx, modes, keys);
  const short = campShortNames(keys);
  const workloads = mergeOrder(keys.map((k) => ctx.data(k)!.order(spec.x)));
  const labels: Record<string, string> = Object.assign({}, ...keys.map((k) => labelsOf(ctx.data(k)!, spec.x)));
  const series: Series[] = keys.map((key) => {
    const d = ctx.data(key)!, bkey = baseSourceKey(ctx, key), bd = ctx.data(bkey), li = ctx.plot.sel.indexOf(key);
    const rest = restOf(d, spec), brest = bd ? restOf(bd, spec) : null;
    return {
      key, hideKey: "camp:" + key, label: `${PLOT_LETTERS[li]} ${short[key]}`, long: t("cmp.long", { name: d.name, host: d.host }),
      pal: "camp", idx: li, src: key, baseSrc: bkey, variant: "", mode: config, gmean: null,
      pairs: Object.fromEntries(workloads.map((w) => [w, pairOf(spec, cellAt(spec, d, w, rest, config), cellAt(spec, bd, w, brest, spec.baseline as string))])),
    };
  });
  const m = finishModel(ctx, {
    type: "bars", id: `compare_${config}`, title: t("cmp.title", { label: specLabel(spec), series: seriesLabel(ctx, keys, config) }),
    workloads, labels, series, legend: true, noun: t("noun.campaign"),
  });
  m.sub = series.map((s) => t("cmp.seriesLine", { label: s.label, long: s.long,
    base: s.baseSrc !== s.src ? t("cmp.baseFrom", { l: letterOf(ctx, s.baseSrc) }) : t("cmp.baseOwn") }));
  const g = gmeanText(m);
  if (g) m.sub.push(g);
  m.warn = [];
  m.diffs = [];
  const src = (k: string): Src => { const c = ctx.data(k); return { key: k, rest: c ? restOf(c, spec) : null, mode: config, variant: "" }; };
  for (const s of series) {
    if (s.baseSrc !== s.src) {
      const pv = provenance(ctx, src(s.src), src(s.baseSrc), false);
      if (pv && !pv.same) m.warn.push(pv.text);
    }
    if (s !== series[0]) {
      const pv = provenance(ctx, src(s.src), src(series[0].src), false);
      if (pv) m.diffs.push(pv.text);
    }
  }
  return { models: [m], modes, config, notes: [] };
}

export interface RefSrc { key: string; rest: Rest | null; variant: string }

export interface SensSetup {
  skeys: string[];
  skey: string;
  d: Campaign;
  mains: string[];
  ref: string;
  refSrc: RefSrc | null;
  fixedKeys: string[];
  fixedKey: string | null;
  fixedSrc: RefSrc | null;
  norm: "fixed" | "matched";
  modes: string[];
  config: string;
}

export function sensitivitySetup(ctx: ChartCtx): SensSetup | { notes: string[] } {
  const spec = specOf(ctx);
  const skeys = ctx.plot.sel.filter((k) => ctx.data(k)?.sweep);
  if (!skeys.length) return { notes: [t("sens.none")] };
  const skey = skeys.includes(ctx.plot.sens) ? ctx.plot.sens : skeys[0];
  const d = ctx.data(skey)!;
  const sens = d.sens ?? { reference: null, refName: null, dims: [] };
  // Other selected campaigns that can supply a reference point / fixed baseline:
  // plain campaigns and sweep campaigns with their own reference point.
  const srcOf = (k: string): RefSrc => {
    const o = ctx.data(k)!;
    if (o.sweep) return { key: k, rest: o.sens?.reference ?? null, variant: o.sens?.refName ?? "" };
    return { key: k, rest: restOf(o, spec), variant: "" };
  };
  const mains = ctx.plot.sel.filter((k) => {
    const o = k !== skey && ctx.data(k);
    if (!o) return false;
    return o.sweep ? !!o.sens?.reference && hasBaseline(ctx, k, o.sens.reference) : hasBaseline(ctx, k);
  });
  // The reference point (the "*" bars): the campaign's own when it ran one,
  // else a selected campaign's (different runs: only as a stand-in).
  let ref = ctx.plot.ref;
  if (ref !== "none") ref = sens.reference ? "own" : mains.includes(ref) ? ref : mains[0] || "none";
  const refSrc = ref === "own" ? srcOf(skey) : ref === "none" ? null : srcOf(ref);
  // Fixed baseline (normalisation "fixed" only): whose baseline divides every point.
  const fixedKeys = (sens.reference ? [skey] : []).concat(mains);
  const fixedKey = fixedKeys.includes(ctx.plot.fixed) ? ctx.plot.fixed : mains[0] || fixedKeys[0] || null;
  const fixedSrc = fixedKey ? srcOf(fixedKey) : null;
  const norm = ctx.plot.norm === "fixed" && fixedSrc ? "fixed" : "matched";
  const modes = plotModes(ctx, [skey], norm === "fixed");
  const config = modes.length ? pickConfig(ctx, modes, [skey]) : ctx.plot.config;
  return { skeys, skey, d, mains, ref, refSrc, fixedKeys, fixedKey, fixedSrc, norm, modes, config };
}

function srcText(ctx: ChartCtx, src: { key: string; variant: string }, skey: string): string {
  if (src.key === skey) return t("sens.srcOwn", { v: src.variant });
  return `${letterOf(ctx, src.key)} ${keyParts(src.key).name}` + (src.variant ? t("sens.srcOf", { v: src.variant }) : "");
}

export function sensitivityModels(ctx: ChartCtx, st: SensSetup): ChartResult {
  const spec = specOf(ctx);
  const { skey, d, refSrc, fixedSrc, norm, config } = st;
  const sens = d.sens ?? { reference: null, refName: null, dims: [] };
  const notes: string[] = [], warns: string[] = [];
  if (ctx.plot.norm === "fixed" && norm !== "fixed") notes.push(t("sens.needFixed"));
  const models: BarModel[] = [];
  const firstPoint = (sens.dims[0] || { points: [] }).points[0];
  const checked = new Set<string>();
  const pairsToCheck: [RefSrc | null, string][] = [[refSrc, t("sens.refWhat")], [norm === "fixed" ? fixedSrc : null, t("sens.fixedWhat")]];
  const base = spec.baseline as string;
  for (const [src, what] of pairsToCheck) {
    if (!src || src.key === skey || !firstPoint || checked.has(src.key + "|" + src.variant)) continue;
    checked.add(src.key + "|" + src.variant);
    const pv = provenance(ctx, { key: skey, rest: firstPoint.cell, mode: base, variant: firstPoint.name },
      { key: src.key, rest: src.rest, mode: base, variant: src.variant }, true);
    if (pv && !pv.same) warns.push(t("sens.fromOther", { what, text: pv.text }));
    else if (pv) notes.push(t("sens.note", { what, text: pv.text }));
    else warns.push(t("sens.cannotCompare", { what, src: srcText(ctx, src, skey) }));
  }
  const workloads = d.order(spec.x), labels = labelsOf(d, spec.x);
  for (const dim of sens.dims) {
    const points: (SensPoint & { key: string; rest: Rest | null; variant: string; ref: boolean })[] = dim.points.map((p) => ({
      ...p, key: skey, rest: p.cell, variant: p.name, ref: false,
    }));
    if (refSrc && (dim.ref_value == null || !points.some((p) => p.value === dim.ref_value))) {
      points.push({ name: refSrc.variant || t("sens.reference"), cell: refSrc.rest ?? {}, value: dim.ref_value, tick: dim.ref_tick + "*",
        legend: t("sens.refLegend", { legend: dim.ref_legend }), key: refSrc.key, rest: refSrc.rest, variant: refSrc.variant, ref: true });
    }
    if (points.every((p) => p.value != null)) points.sort((a, b) => (a.value as number) - (b.value as number));
    const fixed = norm === "fixed" && fixedSrc ? { d: ctx.data(fixedSrc.key), rest: fixedSrc.rest } : null;
    const series: Series[] = points.map((p, i) => {
      const pd = ctx.data(p.key);
      return {
        key: p.name, hideKey: "pt:" + p.name, label: p.legend, tick: p.tick, pal: "seq", idx: i, n: points.length,
        src: p.key, baseSrc: fixed && fixedSrc ? fixedSrc.key : p.key, variant: p.variant, mode: config, pointValue: p.value, ref: p.ref,
        gmean: null,
        pairs: Object.fromEntries(workloads.map((w) => [w, pairOf(spec, cellAt(spec, pd, w, p.rest, config),
          fixed ? cellAt(spec, fixed.d, w, fixed.rest, base) : cellAt(spec, pd, w, p.rest, base))])),
      };
    });
    const m = finishModel(ctx, {
      type: "bars", id: `sensitivity_${dim.name}_${config}_${norm}`,
      title: t("sens.title", { label: dim.title, name: dim.name }), xTitle: dim.xlabel, workloads, labels,
      series, legend: true, dim: dim.name, noun: t("noun.point"),
    });
    m.warn = [];
    const refPoint = points.find((p) => p.ref);
    const cfgLabel = d.label(spec.series, config);
    m.sub = [norm === "fixed" && fixedSrc
      ? t("sens.byFixed", { series: cfgLabel, src: srcText(ctx, fixedSrc, skey) })
      : t("sens.byMatched", { series: cfgLabel })];
    if (refPoint) m.sub.push(t("sens.refPoint", { src: srcText(ctx, refPoint, skey) }) + (refPoint.key === skey ? "" : t("sens.refStandIn")));
    const g = gmeanText(m);
    if (g) m.sub.push(g);
    // GMEAN trend across the swept points (same x set as the bars).
    if (m.summary) {
      m.line = {
        type: "line", id: m.id + "_gmean", title: t("sens.lineTitle", { x: dim.xlabel }), xTitle: dim.xlabel,
        xName: spec.x, yLabel: specLabel(spec),
        points: m.series.map((s) => ({ tick: s.tick ?? "", legend: s.label, v: m.visible.includes(s) ? s.gmean : null,
          hidden: !m.visible.includes(s), ref: s.ref })),
        n: m.common.length, total: m.workloads.length,
      };
    }
    models.push(m);
  }
  if (!sens.dims.length) notes.push(t("sens.noDims"));
  return { models, notes, warns };
}

export type ChartKind = "ablation" | "sensitivity" | "compare";

/** Which chart kinds the selection allows; plot.kind "" = automatic choice. */
export function plotState(ctx: ChartCtx): { kind: ChartKind; avail: Record<string, boolean> } {
  const kinds = ctx.plot.sel.map((k) => kindOf(ctx, k));
  const avail: Record<string, boolean> = {
    ablation: kinds.some((k) => k !== "sensitivity"), sensitivity: kinds.includes("sensitivity"),
    compare: kinds.filter((k) => k !== "sensitivity").length >= 2,
  };
  let kind = ctx.plot.kind;
  if (!kind || !avail[kind]) kind = avail.sensitivity ? "sensitivity" : avail.compare ? "compare" : "ablation";
  return { kind: kind as ChartKind, avail };
}

export interface BuiltCharts extends ChartResult {
  kind: ChartKind;
  avail: Record<string, boolean>;
  sens: SensSetup | null;
  /** The specs that fit the chart kind, and the one drawn. */
  specs: PlotSpec[];
  spec: PlotSpec | null;
}

/** The plot specs for a selection: the first loaded campaign's project's, else derived from its cells. */
export function selectionSpecs(sel: string[], data: (k: string) => Campaign | null,
  projectSpecs: (project: string) => PlotSpec[] | null | undefined): { specs: PlotSpec[]; derived: boolean } {
  for (const k of sel) {
    const s = projectSpecs(keyParts(k).project);
    if (s?.length) return { specs: s, derived: false };
  }
  for (const k of sel) {
    const s = defaultSpecs(data(k));
    if (s.length) return { specs: s, derived: true };
  }
  return { specs: [], derived: true };
}

/** Everything the chart page draws for the current selection. */
export function buildCharts(ctx: ChartCtx): BuiltCharts {
  const { kind, avail } = plotState(ctx);
  const fit = specsFor(ctx.specs, kind);
  const spec = fit.find((s) => s.name === ctx.plot.spec) ?? fit[0] ?? null;
  if (!spec) return { models: [], notes: [], kind, avail, sens: null, specs: fit, spec: null };
  ctx = { ...ctx, spec };
  const ready = ctx.plot.sel.filter((k) => ctx.data(k));
  let result: ChartResult = { models: [], notes: [] };
  let sens: SensSetup | null = null;
  if (kind === "sensitivity" && ready.some((k) => ctx.data(k)!.sweep)) {
    const st = sensitivitySetup(ctx);
    if ("d" in st) { sens = st; result = sensitivityModels(ctx, st); }
    else result = { models: [], notes: st.notes };
  } else if (kind === "compare" && ready.length) {
    result = compareModel(ctx);
  } else if (kind === "ablation" && ready.length) {
    result = ablationModels(ctx);
  }
  return { ...result, kind, avail, sens, specs: fit, spec };
}

/** A context bound to a spec, for callers outside buildCharts (the chart page's baseline chooser). */
export function withSpec(ctx: ChartCtx, spec: PlotSpec): ChartCtx {
  return { ...ctx, spec };
}

// ---------- CSV ----------

function csvCell(v: unknown): string {
  const s = v == null ? "" : String(v);
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function chartCSV(m: BarModel): string {
  const spec = m.spec, match = spec.match ?? [];
  const src = (k: string) => { const p = keyParts(k); return `${p.host}:${p.name}`; };
  const fields = ["chart", "series", "variant", "point_value", spec.x, "label", spec.series, "flag", "note",
    "baseline_source", `baseline_${spec.metric}`, ...match.map((x) => `baseline_${x}`),
    "source", spec.metric, ...match, spec.name || "value"];
  const rows = [fields.map(csvCell).join(",")];
  const mv = (c: CellDoc | null, k: string) => (c ? c.metrics?.[k] ?? "" : "");
  for (const s of m.visible) {
    for (const w of m.workloads) {
      const p = s.pairs[w];
      rows.push([m.id, s.label, s.variant, s.pointValue, w, m.labels[w] || w, s.mode, p.flag, p.note || "",
        p.base ? src(s.baseSrc) : "", mv(p.base, spec.metric), ...match.map((x) => mv(p.base, x)),
        p.cell ? src(s.src) : "", mv(p.cell, spec.metric), ...match.map((x) => mv(p.cell, x)),
        p.flag === "ok" ? p.s : ""].map(csvCell).join(","));
    }
    if (!m.summary) continue;
    rows.push([m.id, s.label, s.variant, s.pointValue, "GMEAN", "GMEAN", s.mode, s.gmean == null ? "none" : "ok",
      `geomean over ${m.common.length}/${m.workloads.length} ${spec.x} values OK in all ${m.counted.length} series with results` +
        (m.empty.length ? ` (${m.empty.length} series without any result left out)` : "") + `: ${m.common.join(" ")}`,
      "", "", ...match.map(() => ""), "", "", ...match.map(() => ""), s.gmean == null ? "" : s.gmean].map(csvCell).join(","));
  }
  return rows.join("\n") + "\n";
}
