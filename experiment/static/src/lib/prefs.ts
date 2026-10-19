// UI preferences shared by the pages, kept in localStorage (every access wrapped
// in try/catch: private windows and blocked storage must still work).
import { useSyncExternalStore } from "react";
import { defaultPlot, type PlotPrefs } from "./charts/model.ts";
import type { FacetSel } from "./naming.ts";

export interface Prefs {
  // sessions
  sessSearch: string;
  sessHost: string; // "" = all
  sessState: string; // "" = all
  recent: boolean; // only the last 24 h
  grouped: boolean; // fold `exp run -g` groups
  // shared
  showArchived: boolean;
  // results
  resSearch: string;
  facets: FacetSel;
  cmp: string[]; // "host|project|name", up to 4
  /** Result groups folded: every group in `mode`, except those in `flip`. */
  resFold: { mode: "open" | "closed"; flip: string[] };
  // charts
  plot: PlotPrefs;
}

const KEY = "akitaExpUI";
const OLD_KEY = "expMonitorUI"; // the old page's prefs: chart selection and comparison carry over

function defaults(): Prefs {
  return {
    sessSearch: "", sessHost: "", sessState: "", recent: true, grouped: true, showArchived: false,
    resSearch: "", facets: {}, cmp: [], resFold: { mode: "open", flip: [] }, plot: defaultPlot(),
  };
}

function readJSON(key: string): Record<string, unknown> | null {
  try {
    const raw = window.localStorage.getItem(key);
    const v = raw ? JSON.parse(raw) : null;
    return v && typeof v === "object" ? v : null;
  } catch {
    return null;
  }
}

function sanitize(u: Record<string, unknown> | null): Prefs {
  const d = defaults();
  if (!u) return d;
  const p: Prefs = { ...d };
  for (const k of Object.keys(d) as (keyof Prefs)[]) {
    if (k in u && typeof u[k] === typeof d[k]) (p as unknown as Record<string, unknown>)[k] = u[k];
  }
  if (!Array.isArray(p.cmp)) p.cmp = [];
  if (!p.resFold || (p.resFold.mode !== "open" && p.resFold.mode !== "closed") || !Array.isArray(p.resFold.flip)) p.resFold = { mode: "open", flip: [] };
  if (!p.facets || typeof p.facets !== "object" || Array.isArray(p.facets)) p.facets = {};
  const plot = { ...defaultPlot(), ...(u.plot && typeof u.plot === "object" ? (u.plot as object) : {}) } as PlotPrefs;
  if (!Array.isArray(plot.sel)) plot.sel = [];
  if (!Array.isArray(plot.hide)) plot.hide = [];
  if (!plot.base || typeof plot.base !== "object") plot.base = {};
  plot.cap = Number(plot.cap) || 0;
  for (const k of ["kind", "spec", "config", "ref", "norm", "fixed", "sens"] as const) if (typeof plot[k] !== "string") plot[k] = defaultPlot()[k];
  p.plot = plot;
  return p;
}

function load(): Prefs {
  const cur = readJSON(KEY);
  if (cur) return sanitize(cur);
  const old = readJSON(OLD_KEY);
  const p = defaults();
  if (old) {
    const s = sanitize({ plot: old.plot, cmp: old.cmp });
    p.plot = s.plot;
    p.cmp = s.cmp;
  }
  return p;
}

let state: Prefs = typeof window === "undefined" ? defaults() : load();

// Shareable links: ?plot=host|project|name,... and ?cmp=... preselect.
if (typeof window !== "undefined") {
  try {
    const q = new URLSearchParams(window.location.search);
    const plot = q.get("plot"), cmp = q.get("cmp");
    if (plot) state = { ...state, plot: { ...state.plot, sel: plot.split(",").filter((k) => k.split("|").length === 3).slice(0, 4), kind: "", hide: [], base: {} } };
    if (cmp) state = { ...state, cmp: cmp.split(",").filter((k) => k.split("|").length === 3).slice(0, 4) };
  } catch { /* ignore */ }
}

const listeners = new Set<() => void>();

export function getPrefs(): Prefs {
  return state;
}

export function setPrefs(patch: Partial<Prefs> | ((p: Prefs) => Partial<Prefs>)): void {
  const next = typeof patch === "function" ? patch(state) : patch;
  state = { ...state, ...next };
  try { window.localStorage.setItem(KEY, JSON.stringify(state)); } catch { /* ignore */ }
  listeners.forEach((l) => l());
}

export function setPlot(patch: Partial<PlotPrefs> | ((p: PlotPrefs) => Partial<PlotPrefs>)): void {
  setPrefs((s) => ({ plot: { ...s.plot, ...(typeof patch === "function" ? patch(s.plot) : patch) } }));
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function usePrefs(): Prefs {
  return useSyncExternalStore(subscribe, getPrefs, getPrefs);
}

// ---------- selection actions (shared by Results and Charts) ----------

export const MAX_PLOT = 4;

/** "Plot": plot only this campaign. */
export function plotOnly(key: string): void {
  setPlot({ sel: [key], base: {}, hide: [], kind: "" });
}

/** "Add to chart": add or remove a campaign; false when the chart is full. */
export function togglePlot(key: string): boolean {
  const sel = state.plot.sel;
  if (sel.includes(key)) {
    const base = { ...state.plot.base };
    delete base[key];
    setPlot({ sel: sel.filter((k) => k !== key), base, kind: "" });
    return true;
  }
  if (sel.length >= MAX_PLOT) return false;
  setPlot({ sel: [...sel, key], kind: "" });
  return true;
}

/** "Plot the group": replace the selection with these campaigns. */
export function plotMany(keys: string[]): void {
  setPlot({ sel: keys.slice(0, MAX_PLOT), base: {}, hide: [], kind: "" });
}

export function toggleCmp(key: string): boolean {
  const cmp = state.cmp;
  if (cmp.includes(key)) { setPrefs({ cmp: cmp.filter((k) => k !== key) }); return true; }
  if (cmp.length >= 4) return false;
  setPrefs({ cmp: [...cmp, key] });
  return true;
}

export function isGroupOpen(p: Prefs, key: string): boolean {
  return (p.resFold.mode === "open") !== p.resFold.flip.includes(key);
}

export function toggleGroup(key: string): void {
  const f = state.resFold;
  setPrefs({ resFold: { mode: f.mode, flip: f.flip.includes(key) ? f.flip.filter((k) => k !== key) : [...f.flip, key] } });
}

export function foldAll(open: boolean): void {
  setPrefs({ resFold: { mode: open ? "open" : "closed", flip: [] } });
}
