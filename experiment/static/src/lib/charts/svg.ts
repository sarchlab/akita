// SVG strings for the chart models (port of the old page's barsV / barsH /
// lineChart / chartSVG), styled after Daisen's chartStyle.ts: #475569 axis
// labels, dashed 3,3 grid at 0.3 opacity, Akita primary blue for one series and
// a validated colour-blind-safe categorical palette (Okabe-Ito order) for more.
import { t } from "../i18n.ts";
import { keyParts } from "../types.ts";
import { flagInfo, FLAG_ORDER, fmtMetric, nounOf, specLabel, type BarModel, type ChartModel, type LineModel, type Series } from "./model.ts";

export interface Theme {
  surface: string; text: string; muted: string; grid: string; gridOpacity: number; axis: string;
  ref: string; crit: string; primary: string;
  camp: string[]; seq: string[];
  /** The categorical palette in order (also used for compare_by colours outside charts). */
  categorical: string[];
}

export const THEMES: Record<"light" | "dark", Theme> = {
  light: {
    surface: "#ffffff", text: "#0f172a", muted: "#475569", grid: "#000000", gridOpacity: 0.3, axis: "#64748b",
    ref: "#334155", crit: "#dc2626", primary: "#0e5eb3",
    camp: ["#0072B2", "#E69F00", "#009E73", "#D55E00"],
    seq: ["#86b6ef", "#5598e7", "#2a78d6", "#1c5cab", "#104281"],
    categorical: ["#0072B2", "#E69F00", "#009E73", "#D55E00", "#CC79A7", "#56B4E9"],
  },
  dark: {
    surface: "#0f172a", text: "#e2e8f0", muted: "#94a3b8", grid: "#ffffff", gridOpacity: 0.22, axis: "#64748b",
    ref: "#cbd5e1", crit: "#f87171", primary: "#3987e5",
    camp: ["#3987e5", "#c98500", "#199e70", "#d95926"],
    seq: ["#256abf", "#3987e5", "#6da7ec", "#9ec5f4", "#cde2fb"],
    categorical: ["#3987e5", "#c98500", "#199e70", "#d95926", "#9085e9", "#d55181"],
  },
};

const FONT = "Asap, ui-sans-serif, system-ui, sans-serif";
const SEQ_PICK: Record<number, number[]> = { 1: [2], 2: [0, 4], 3: [0, 2, 4], 4: [0, 1, 3, 4], 5: [0, 1, 2, 3, 4] };

export interface Tip { sw: string; v: string; name: string; lines: string[]; crit?: boolean }

export function esc(s: unknown): string {
  return String(s ?? "").replace(/[&<>"']/g,
    (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);
}

function hexRGB(h: string): number[] { return [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16)); }

function seqColor(th: Theme, i: number, n: number): string {
  if (n <= 5) return th.seq[SEQ_PICK[n][i]];
  const pos = (i / (n - 1)) * (th.seq.length - 1), lo = Math.min(Math.floor(pos), th.seq.length - 2), t = pos - lo;
  const a = hexRGB(th.seq[lo]), b = hexRGB(th.seq[lo + 1]);
  return "#" + a.map((v, j) => Math.round(v + (b[j] - v) * t).toString(16).padStart(2, "0")).join("");
}

/** Colour of a series; a chart with a single series uses Akita's primary blue. */
export function seriesColor(th: Theme, s: Series, single = false): string {
  if (s.colorIdx != null) return th.categorical[s.colorIdx % th.categorical.length];
  if (single) return th.primary;
  if (s.pal === "seq") return seqColor(th, s.idx, s.n ?? 1);
  const pal = s.pal === "camp" ? th.camp : th.categorical;
  return pal[s.idx % pal.length];
}

function colorOf(th: Theme, m: BarModel, s: Series): string {
  return seriesColor(th, s, m.series.length === 1);
}

function textW(s: string, size: number): number {
  let w = 0;
  for (const ch of String(s)) w += ch.charCodeAt(0) > 0x2e80 ? size : size * 0.6;
  return w;
}
function niceStep(range: number, count: number): number {
  const raw = range / Math.max(1, count), p = Math.pow(10, Math.floor(Math.log10(raw))), f = raw / p;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * p;
}
function tickFmt(step: number): (v: number) => string {
  let dec = 0;
  while (dec < 4 && Math.abs(step * 10 ** dec - Math.round(step * 10 ** dec)) > 1e-6) dec++;
  return (v) => v.toFixed(dec);
}
const r1 = (v: number) => Math.round(v * 10) / 10;

function gridLine(th: Theme, x1: number, x2: number, y1: number, y2: number, zero: boolean): string {
  return zero
    ? `<line x1="${r1(x1)}" x2="${r1(x2)}" y1="${r1(y1)}" y2="${r1(y2)}" stroke="${th.axis}" stroke-width="1"/>`
    : `<line x1="${r1(x1)}" x2="${r1(x2)}" y1="${r1(y1)}" y2="${r1(y2)}" stroke="${th.grid}" stroke-opacity="${th.gridOpacity}" stroke-dasharray="3,3" stroke-width="1"/>`;
}

export function barScale(m: BarModel): { hi: number; clipAt: number | null; ticks: number[]; fmt: (v: number) => string } {
  let vmax = 1;
  for (const s of m.visible) {
    for (const w of m.workloads) if (s.pairs[w].flag === "ok") vmax = Math.max(vmax, s.pairs[w].s as number);
    if (s.gmean != null) vmax = Math.max(vmax, s.gmean);
  }
  const clipAt = m.cap && vmax > m.cap ? m.cap : null;
  const top = clipAt || vmax * 1.06;
  const step = niceStep(top, 5);
  const hi = clipAt || Math.ceil(top / step - 1e-9) * step;
  const ticks: number[] = [];
  for (let v = 0; v <= hi + step * 1e-6; v += step) ticks.push(+v.toFixed(10));
  return { hi, clipAt, ticks, fmt: tickFmt(step) };
}

interface DrawCell { w: string | null; s: Series; v: number | null; flag: string }

function barCells(m: BarModel): DrawCell[][] {
  const groups: (string | null)[] = m.summary ? [...m.workloads, null] : [...m.workloads];
  return groups.map((w) => m.visible.map((s) => {
    if (w == null) return { w, s, v: s.gmean, flag: s.gmean == null ? "none" : "ok" };
    const p = s.pairs[w];
    return { w, s, v: p.flag === "ok" ? (p.s as number) : null, flag: p.flag };
  }));
}

function svgOpen(W: number, H: number, title: string, exportMode: boolean, th: Theme): string {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${r1(W)} ${r1(H)}" width="${r1(W)}" height="${r1(H)}" role="img" font-family="${FONT}"><title>${esc(title)}</title>` +
    (exportMode ? `<rect width="100%" height="100%" fill="${th.surface}"/>` : "");
}

interface Draw { th: Theme; cid: number; tips: Tip[]; exportMode: boolean }

function hitAttrs(dr: Draw, tip: Tip): string | null {
  dr.tips.push(tip);
  return dr.exportMode ? null : `class="hit" tabindex="0" data-tip="${dr.cid}:${dr.tips.length - 1}"`;
}

function glyphSVG(x: number, y: number, flag: string, th: Theme, anchor?: string): string {
  const f = flagInfo(flag);
  if (!f) return "";
  return `<text x="${r1(x)}" y="${r1(y)}" text-anchor="${anchor || "middle"}" font-size="12" font-weight="700" fill="${f.crit ? th.crit : th.muted}">${f.glyph}</text>`;
}

function pairTip(m: BarModel, s: Series, w: string, th: Theme): Tip {
  const p = s.pairs[w];
  const spec = m.spec, metric = spec.metric;
  const tip: Tip = { sw: colorOf(th, m, s), name: `${s.label} · ${m.labels[w] || w}`, lines: [], v: "" };
  const bv = p.base?.metrics?.[metric], cv = p.cell?.metrics?.[metric];
  if (p.flag === "ok" && p.base && p.cell) {
    tip.v = `${(p.s as number).toFixed(3)}×`;
    tip.lines.push(`baseline ${fmtMetric(metric, bv)} → ${fmtMetric(metric, cv)}`);
    for (const x of spec.match ?? []) tip.lines.push(t("svg.match", { metric: x, v: fmtMetric(x, p.cell.metrics?.[x]) }));
  } else {
    const f = flagInfo(p.flag, spec.match);
    tip.v = f?.text || p.flag;
    tip.crit = !!f?.crit;
    if (p.note) tip.lines.push(p.note);
    if (p.flag === "mismatch" && bv && cv) tip.lines.push(t("svg.ratio", { v: (spec.lower_is_better ? bv / cv : cv / bv).toFixed(3) }));
  }
  if (s.baseSrc !== s.src) tip.lines.push(t("svg.baseFrom", { name: keyParts(s.baseSrc).name }));
  if (p.cell) tip.lines.push(t("svg.cell", { key: Object.values(p.cell.key).filter(Boolean).join(" · ") }));
  return tip;
}

function gmeanTip(m: BarModel, s: Series, th: Theme): Tip {
  return {
    sw: colorOf(th, m, s), name: `${s.label} · GMEAN`,
    v: s.gmean == null ? t("svg.noCommon", { x: m.spec.x }) : `${s.gmean.toFixed(3)}×`,
    lines: [t("svg.gmeanOf", { n: m.counted.length, noun: nounOf(m), k: m.common.length, total: m.workloads.length, x: m.spec.x })],
  };
}

function barsV(m: BarModel, dr: Draw, W: number) {
  const th = dr.th;
  const cells = barCells(m), nS = m.visible.length, sc = barScale(m);
  const lab = (w: string | null) => (w == null ? "GMEAN" : m.labels[w] || w);
  const ml = 44, mr = 8, plotH = 240;
  const bw = (W - ml - mr) / cells.length;
  const barW = Math.max(1.5, Math.min(24, (bw * 0.8 - 2 * (nS - 1)) / nS));
  const gW = nS * barW + 2 * (nS - 1);
  const maxLab = Math.max(...cells.map((_, gi) => textW(lab(gi < m.workloads.length ? m.workloads[gi] : null), 11)));
  const rot = maxLab + 6 > bw;
  const mb = rot ? Math.min(150, maxLab * 0.64 + 22) : 26;
  const vRot = barW + 2 < textW("8.88", 10);
  const mt = vRot ? 40 : 24;
  const H = mt + plotH + mb;
  const y = (v: number) => mt + plotH - (Math.min(v, sc.hi) / sc.hi) * plotH;
  const base = mt + plotH;
  const out: string[] = [], over: string[] = [], hits: string[] = [];
  for (const t of sc.ticks) {
    out.push(gridLine(th, ml, W - mr, y(t), y(t), t === 0));
    out.push(`<text x="${ml - 6}" y="${r1(y(t) + 4)}" text-anchor="end" font-size="10" fill="${th.muted}">${sc.fmt(t)}</text>`);
  }
  out.push(`<text x="0" y="12" font-size="11" fill="${th.muted}">${esc(t("svg.axis", { label: specLabel(m.spec) }))}</text>`);
  const sepX = ml + bw * m.workloads.length;
  if (m.summary) out.push(gridLine(th, sepX, sepX, mt, base, false));
  cells.forEach((group, gi) => {
    const x0 = ml + gi * bw + (bw - gW) / 2;
    group.forEach((c, si) => {
      const x = x0 + si * (barW + 2), cx = x + barW / 2, color = colorOf(th, m, c.s);
      if (c.v != null) {
        const top = y(c.v), h = base - top, r = Math.min(4, barW / 2, h);
        out.push(`<path d="M${r1(x)} ${base}V${r1(top + r)}Q${r1(x)} ${r1(top)} ${r1(x + r)} ${r1(top)}H${r1(x + barW - r)}Q${r1(x + barW)} ${r1(top)} ${r1(x + barW)} ${r1(top + r)}V${base}Z" fill="${color}"/>`);
        const clipped = sc.clipAt && c.v > sc.clipAt;
        if (clipped) over.push(`<path d="M${r1(x - 1)} ${r1(top + 11)}L${r1(x + barW + 1)} ${r1(top + 6)}M${r1(x - 1)} ${r1(top + 16)}L${r1(x + barW + 1)} ${r1(top + 11)}" stroke="${th.surface}" stroke-width="2"/>`);
        if (c.w == null || clipped) {
          const text = c.v.toFixed(2);
          over.push(vRot
            ? `<text transform="translate(${r1(cx + 3.5)} ${r1(top - 4)}) rotate(-90)" font-size="10" fill="${th.text}">${text}</text>`
            : `<text x="${r1(cx)}" y="${r1(top - 4)}" text-anchor="middle" font-size="10" fill="${th.text}">${text}</text>`);
        }
      } else {
        over.push(glyphSVG(cx, base - 4, c.w != null ? c.flag : "missing", th));
      }
      const attrs = hitAttrs(dr, c.w == null ? gmeanTip(m, c.s, th) : pairTip(m, c.s, c.w, th));
      if (attrs) hits.push(`<rect ${attrs} x="${r1(x - 1)}" y="${mt}" width="${r1(barW + 2)}" height="${plotH}"/>`);
    });
    const gx = ml + gi * bw + bw / 2, w = gi < m.workloads.length ? m.workloads[gi] : null;
    const weight = w == null ? ` font-weight="600" fill="${th.text}"` : ` fill="${th.muted}"`;
    out.push(rot
      ? `<text transform="translate(${r1(gx + 3)} ${base + 10}) rotate(-40)" text-anchor="end" font-size="11"${weight}>${esc(lab(w))}</text>`
      : `<text x="${r1(gx)}" y="${base + 17}" text-anchor="middle" font-size="11"${weight}>${esc(lab(w))}</text>`);
  });
  if (1 <= sc.hi) over.push(`<line x1="${ml}" x2="${r1(W - mr)}" y1="${r1(y(1))}" y2="${r1(y(1))}" stroke="${th.ref}" stroke-width="1" stroke-dasharray="4 3"/>`);
  return { W, H, body: out.join("") + over.join("") + hits.join("") };
}

function barsH(m: BarModel, dr: Draw, W: number) {
  const th = dr.th;
  const cells = barCells(m), nS = m.visible.length, sc = barScale(m);
  const lab = (w: string | null) => (w == null ? "GMEAN" : m.labels[w] || w);
  const barT = nS <= 2 ? 12 : nS <= 4 ? 9 : 7;
  const rowH = nS * barT + 2 * (nS - 1) + 12;
  const maxLab = Math.max(...m.workloads.map((w) => textW(lab(w), 11)), textW("GMEAN", 11));
  const ml = Math.min(W * 0.42, maxLab + 10), mr = 34, mt = 34, mb = 6;
  const plotW = W - ml - mr, H = mt + cells.length * rowH + mb;
  const x = (v: number) => ml + (Math.min(v, sc.hi) / sc.hi) * plotW;
  const out: string[] = [], over: string[] = [], hits: string[] = [];
  out.push(`<text x="0" y="12" font-size="11" fill="${th.muted}">${esc(t("svg.axis", { label: specLabel(m.spec) }))}</text>`);
  for (const t of sc.ticks) {
    out.push(gridLine(th, x(t), x(t), mt - 4, H - mb, t === 0));
    out.push(`<text x="${r1(x(t))}" y="${mt - 9}" text-anchor="middle" font-size="10" fill="${th.muted}">${sc.fmt(t)}</text>`);
  }
  const sepY = mt + rowH * m.workloads.length;
  if (m.summary) out.push(gridLine(th, 0, W, sepY, sepY, false));
  cells.forEach((group, gi) => {
    const y0 = mt + gi * rowH + 6, w = gi < m.workloads.length ? m.workloads[gi] : null;
    const weight = w == null ? ` font-weight="600" fill="${th.text}"` : ` fill="${th.muted}"`;
    out.push(`<text x="${r1(ml - 6)}" y="${r1(y0 + (rowH - 12) / 2 + 4)}" text-anchor="end" font-size="11"${weight}>${esc(lab(w))}</text>`);
    group.forEach((c, si) => {
      const by = y0 + si * (barT + 2), color = colorOf(th, m, c.s);
      if (c.v != null) {
        const end = x(c.v), len = end - ml, r = Math.min(4, barT / 2, len);
        out.push(`<path d="M${r1(ml)} ${r1(by)}H${r1(end - r)}Q${r1(end)} ${r1(by)} ${r1(end)} ${r1(by + r)}V${r1(by + barT - r)}Q${r1(end)} ${r1(by + barT)} ${r1(end - r)} ${r1(by + barT)}H${r1(ml)}Z" fill="${color}"/>`);
        const clipped = sc.clipAt && c.v > sc.clipAt;
        if (clipped) over.push(`<path d="M${r1(end - 11)} ${r1(by + barT + 1)}L${r1(end - 6)} ${r1(by - 1)}M${r1(end - 16)} ${r1(by + barT + 1)}L${r1(end - 11)} ${r1(by - 1)}" stroke="${th.surface}" stroke-width="2"/>`);
        if (c.w == null || clipped) over.push(`<text x="${r1(end + 4)}" y="${r1(by + barT / 2 + 3.5)}" font-size="10" fill="${th.text}">${c.v.toFixed(2)}</text>`);
      } else {
        over.push(glyphSVG(ml + 4, by + barT / 2 + 4, c.w == null ? "missing" : c.flag, th, "start"));
      }
      const attrs = hitAttrs(dr, c.w == null ? gmeanTip(m, c.s, th) : pairTip(m, c.s, c.w, th));
      if (attrs) hits.push(`<rect ${attrs} x="${r1(ml)}" y="${r1(by - 1)}" width="${r1(plotW)}" height="${barT + 2}"/>`);
    });
  });
  if (1 <= sc.hi) over.push(`<line x1="${r1(x(1))}" x2="${r1(x(1))}" y1="${mt - 4}" y2="${H - mb}" stroke="${th.ref}" stroke-width="1" stroke-dasharray="4 3"/>`);
  return { W, H, body: out.join("") + over.join("") + hits.join("") };
}

function lineChart(lm: LineModel, dr: Draw, W: number) {
  const th = dr.th;
  const pts = lm.points, n = pts.length;
  const vals = pts.map((p) => p.v).filter((v): v is number => v != null);
  let lo = Math.min(1, ...vals), hi = Math.max(1, ...vals);
  const pad = Math.max(0.03, (hi - lo) * 0.15);
  const step = niceStep(hi - lo + 2 * pad, 4);
  lo = Math.max(0, Math.floor((lo - pad) / step) * step);
  hi = Math.ceil((hi + pad) / step) * step;
  const fmt = tickFmt(step);
  const ml = 40, mr = 14, mt = 22, plotH = 150;
  const maxTick = Math.max(...pts.map((p) => textW(p.tick, 11)));
  const plotW = W - ml - mr, slot = plotW / Math.max(1, n);
  const rot = maxTick + 6 > slot;
  const mb = (rot ? maxTick * 0.64 + 16 : 22) + 18;
  const H = mt + plotH + mb;
  const x = (i: number) => ml + (i + 0.5) * slot, y = (v: number) => mt + ((hi - v) / (hi - lo)) * plotH;
  const out: string[] = [], over: string[] = [], hits: string[] = [];
  for (let t = lo; t <= hi + step * 1e-6; t += step) {
    const tv = +t.toFixed(10);
    out.push(gridLine(th, ml, W - mr, y(tv), y(tv), false));
    out.push(`<text x="${ml - 6}" y="${r1(y(tv) + 4)}" text-anchor="end" font-size="10" fill="${th.muted}">${fmt(tv)}</text>`);
  }
  out.push(`<text x="0" y="12" font-size="11" fill="${th.muted}">${esc(t("svg.lineAxis", { label: lm.yLabel }))}</text>`);
  if (lo <= 1 && 1 <= hi) out.push(`<line x1="${ml}" x2="${r1(W - mr)}" y1="${r1(y(1))}" y2="${r1(y(1))}" stroke="${th.ref}" stroke-width="1" stroke-dasharray="4 3"/>`);
  let path = "", pen = false;
  pts.forEach((p, i) => {
    if (p.v == null) { pen = false; return; }
    path += `${pen ? "L" : "M"}${r1(x(i))} ${r1(y(p.v))}`;
    pen = true;
  });
  if (path) out.push(`<path d="${path}" fill="none" stroke="${th.primary}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`);
  pts.forEach((p, i) => {
    const cx = x(i);
    if (p.v != null) {
      over.push(`<circle cx="${r1(cx)}" cy="${r1(y(p.v))}" r="4" fill="${th.primary}" stroke="${th.surface}" stroke-width="2"/>`);
      over.push(`<text x="${r1(cx)}" y="${r1(y(p.v) - 9)}" text-anchor="middle" font-size="10" fill="${th.text}">${p.v.toFixed(2)}</text>`);
    } else {
      over.push(glyphSVG(cx, mt + plotH - 4, "missing", th));
    }
    const weight = p.ref ? ` font-weight="600" fill="${th.text}"` : ` fill="${th.muted}"`;
    out.push(rot
      ? `<text transform="translate(${r1(cx + 3)} ${mt + plotH + 10}) rotate(-40)" text-anchor="end" font-size="11"${weight}>${esc(p.tick)}</text>`
      : `<text x="${r1(cx)}" y="${mt + plotH + 16}" text-anchor="middle" font-size="11"${weight}>${esc(p.tick)}</text>`);
    const attrs = hitAttrs(dr, {
      sw: th.primary, v: p.v == null ? (p.hidden ? t("svg.hidden") : t("svg.noCommon", { x: lm.xName })) : `${p.v.toFixed(3)}×`,
      name: `${p.legend} · GMEAN`, lines: [t("svg.lineTip", { n: lm.n, total: lm.total, x: lm.xName })],
    });
    if (attrs) hits.push(`<rect ${attrs} x="${r1(cx - Math.min(slot / 2, 24))}" y="${mt}" width="${r1(Math.min(slot, 48))}" height="${plotH}"/>`);
  });
  out.push(`<text x="${r1(ml + plotW / 2)}" y="${r1(H - 4)}" text-anchor="middle" font-size="11" fill="${th.muted}">${esc(lm.xTitle || "")}</text>`);
  return { W, H, body: out.join("") + over.join("") + hits.join("") };
}

/**
 * The SVG of one chart. `tips` collects the tooltip of each hover target
 * (data-tip="cid:index"). Export mode adds a background, the legend and the
 * meaning of the marks, and no hover targets.
 */
export function chartSVG(m: ChartModel, th: Theme, W: number, cid: number, tips: Tip[], horiz: boolean, exportMode = false): string {
  const dr: Draw = { th, cid, tips, exportMode };
  const r = m.type === "line" ? lineChart(m, dr, W) : horiz ? barsH(m, dr, W) : barsV(m, dr, W);
  if (!exportMode || m.type !== "bars") return svgOpen(r.W, r.H, m.title, exportMode, th) + r.body + "</svg>";
  const items = m.visible.length > 1 ? m.visible.map((s) => [colorOf(th, m, s), s.label]) : [];
  const legend: string[] = [];
  let x = 44, row = 0;
  for (const [color, label] of items) {
    const w = 16 + textW(label, 11) + 16;
    if (x + w > W && x > 44) { x = 44; row++; }
    legend.push(`<rect x="${r1(x)}" y="${6 + row * 18}" width="10" height="10" rx="2" fill="${color}"/><text x="${r1(x + 16)}" y="${15 + row * 18}" font-size="11" fill="${th.text}">${esc(label)}</text>`);
    x += w;
  }
  const legH = items.length ? (row + 1) * 18 + 8 : 0;
  const marks = FLAG_ORDER.filter((f) => m.flagCounts[f]).map((f) => {
    const fi = flagInfo(f, m.spec.match);
    return `${fi?.glyph ?? ""} ${fi?.text ?? f} ${m.flagCounts[f]}`;
  });
  const gm = m.summary && m.common.length ? t("svg.exportGmean", { k: m.common.length, total: m.workloads.length, x: m.spec.x, noun: nounOf(m) }) +
    (m.empty.length ? t("svg.exportEmpty", { names: m.empty.map((s) => s.label).join(t("common.list")) }) : "") : "";
  const notes = [gm, marks.join(" · ")].filter(Boolean);
  const noteH = notes.length * 16 + 4;
  return svgOpen(r.W, r.H + legH + noteH, m.title, true, th) + legend.join("") +
    `<g transform="translate(0 ${legH})">${r.body}</g>` +
    notes.map((t, i) => `<text x="0" y="${r1(r.H + legH + 12 + i * 16)}" font-size="11" fill="${th.muted}">${esc(t)}</text>`).join("") + "</svg>";
}

/** A standalone SVG file of a chart (light theme, like the old page). */
export function exportSVG(m: ChartModel): string {
  const W = m.type === "line" ? 420 : Math.max(720, 64 * (m.workloads.length + (m.summary ? 1 : 0)));
  return `<?xml version="1.0" encoding="UTF-8"?>\n${chartSVG(m, THEMES.light, W, 0, [], false, true)}\n`;
}
