// Formatting helpers (ported from the old page).
import { intlLocale } from "./i18n.ts";

export function fmtDur(s: number | null | undefined): string {
  if (s == null || s < 0 || !Number.isFinite(s)) return "—";
  s = Math.round(s);
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600),
    m = Math.floor((s % 3600) / 60), sec = s % 60;
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : m ? `${m}m ${sec}s` : `${sec}s`;
}

export function fmtTime(t: number | null | undefined): string {
  if (!t) return "—";
  return new Date(t * 1000).toLocaleString(intlLocale(), {
    month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
  });
}

/** Local "MM/DD HH:MM" for dense lists. */
export function fmtTimeShort(t: number | null | undefined): string {
  if (!t) return "—";
  return new Date(t * 1000).toLocaleString(intlLocale(), { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
}

export function utcTitle(t: number | null | undefined): string {
  return t ? `UTC ${new Date(t * 1000).toISOString().replace("T", " ").slice(0, 19)}` : "";
}

/** Simulated time given in nanoseconds. */
export function fmtSim(ns: number | null | undefined): string {
  if (ns == null || !Number.isFinite(ns)) return "—";
  const a = Math.abs(ns);
  return a >= 1e9 ? (ns / 1e9).toFixed(3) + " s" : a >= 1e6 ? (ns / 1e6).toFixed(2) + " ms"
    : a >= 1e3 ? (ns / 1e3).toFixed(1) + " µs" : ns.toFixed(0) + " ns";
}

export function fmtSize(b: number): string {
  return b > 1 << 20 ? (b / (1 << 20)).toFixed(1) + " MB" : b > 1024 ? (b / 1024).toFixed(0) + " KB" : b + " B";
}

export function fmtNum(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "—";
  if (v !== 0 && (Math.abs(v) >= 1e7 || Math.abs(v) < 1e-3)) return v.toExponential(4);
  return Number.isInteger(v) ? String(v) : v.toPrecision(7).replace(/\.?0+$/, "");
}

/** ISO date string to Unix seconds. */
export function iso(t: string | undefined | null): number | null {
  if (!t) return null;
  const v = Date.parse(t);
  return Number.isNaN(v) ? null : v / 1000;
}

export function matchesSearch(q: string, ...fields: unknown[]): boolean {
  const s = q.trim().toLowerCase();
  if (!s) return true;
  return fields.some((f) => String(f ?? "").toLowerCase().includes(s));
}

/**
 * A property value that looks like a time (ISO, "YYYY-MM-DD HH:MM:SS(.fff)" or
 * Unix seconds) as local "MM/DD HH:MM:SS"; null when it is not one.
 */
export function fmtTimeLike(v: string): string | null {
  const s = v.trim();
  let t: number | null = null;
  const m = /^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}(?::\d{2})?)(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$/.exec(s);
  if (m) {
    const ms = Date.parse(`${m[1]}T${m[2]}${m[3] ? m[3].slice(0, 4) : ""}${m[4] ?? ""}`);
    t = Number.isNaN(ms) ? null : ms / 1000;
  } else if (/^\d{10}(\.\d+)?$/.test(s)) {
    const n = Number(s);
    if (n >= 1e9 && n < 4.2e9) t = n;
  }
  return t == null ? null : fmtTime(t);
}
