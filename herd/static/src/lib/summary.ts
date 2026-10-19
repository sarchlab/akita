// summary.csv, run configuration and statistics helpers (ported from the old page).
import { fmtNum, fmtSim } from "./format.ts";
import { t } from "./i18n.ts";
import type { RunDetail } from "./fetchCache.ts";
import type { Run } from "./types.ts";

/**
 * Output-path flags (…-file, …-file-name, …-path, …-out, …-dir) differ between
 * any two runs and say nothing about the configuration.
 */
export function isOutputFlag(name: string): boolean {
  return /(^|-)(file|file-name|path|out|dir)$/.test(name);
}

export function csvRows(text: string | null | undefined): string[][] {
  if (!text) return [];
  return text.trim().split(/\r?\n/).map((l) => l.split(","));
}

/** Short key results for a row (up to 4): the run's metrics, else its summary.csv. */
export function keyResults(run: Pick<Run, "metrics" | "summary">): string[] {
  const metrics = Object.entries(run.metrics ?? {});
  if (metrics.length) {
    return metrics.slice(0, 4).map(([k, v]) => `${k} ${/time/i.test(k) ? fmtSim(v * 1e9) : fmtNum(v)}`);
  }
  const lines = csvRows(run.summary);
  const out: string[] = [];
  if (lines[0]?.[0] === "metric") {
    for (const [metric, where, value] of lines.slice(1)) {
      const v = parseFloat(value);
      if (metric === "total_time" && where === "Driver") out.push(`total ${fmtSim(v * 1e9)}`);
      const m = /^probe_pass(\d+)_ns$/.exec(metric || "");
      if (m) out.push(`${t("summary.pass", { n: m[1] })} ${fmtSim(v)}`);
    }
  } else if (lines[0]?.[0] === "pass") {
    const idx = lines[0].indexOf("elapsed_ns");
    for (const row of lines.slice(1)) out.push(`${t("summary.pass", { n: row[0] })} ${fmtSim(parseFloat(row[idx]))}`);
  }
  return out.slice(0, 4);
}

/** [{label, ns}] from either summary.csv format, for comparisons. */
export function summaryValues(summary: string | null | undefined): { label: string; ns: number }[] {
  const lines = csvRows(summary);
  const out: { label: string; ns: number }[] = [];
  if (lines[0]?.[0] === "metric") {
    for (const [metric, where, value] of lines.slice(1)) {
      const v = parseFloat(value);
      const m = /^probe_pass(\d+)_ns$/.exec(metric || "");
      if (m) out.push({ label: t("summary.probePass", { n: m[1] }), ns: v });
      else if (where === "Driver" && (metric === "total_time" || metric === "kernel_time")) out.push({ label: metric, ns: v * 1e9 });
    }
  } else if (lines[0]?.[0] === "pass") {
    const idx = lines[0].indexOf("elapsed_ns");
    for (const row of lines.slice(1)) out.push({ label: t("summary.probePass", { n: row[0] }), ns: parseFloat(row[idx]) });
  }
  return out;
}

/** Effective configuration: config_used.json when present, else the meta args. */
export function flagsOf(detail: RunDetail): Record<string, string> {
  if (detail.config && detail.config.flags) return detail.config.flags;
  const flags: Record<string, string> = {};
  const args = ((detail.meta && (detail.meta as Record<string, unknown>).args) || []) as string[];
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    if (!a.startsWith("-")) continue;
    const [name, value] = a.replace(/^-+/, "").split(/=(.*)/s);
    if (value !== undefined) flags[name] = value;
    else if (i + 1 < args.length && !args[i + 1].startsWith("-")) flags[name] = args[++i];
    else flags[name] = "true";
  }
  return flags;
}

export function ratio(v: number | null | undefined, base: number | null | undefined): { text: string; cls: "up" | "down" | "eq" } | null {
  if (v == null || base == null || !Number.isFinite(v) || !Number.isFinite(base) || base === 0) return null;
  const r = v / base;
  return { text: `×${r.toFixed(r >= 10 ? 1 : 3)}`, cls: Math.abs(r - 1) < 1e-12 ? "eq" : r > 1 ? "up" : "down" };
}

export interface StatDiff { k: string; vals: (number | null)[]; rel: number }

/** Statistics whose sums differ from A, largest relative difference first. */
export function statDiffs(ms: NonNullable<RunDetail["metrics"]>[]): { rows: StatDiff[]; keys: number } {
  const keys = [...new Set(ms.flatMap((m) => Object.keys(m.agg)))];
  const rows: StatDiff[] = [];
  for (const k of keys) {
    const vals = ms.map((m) => (m.agg[k] ? m.agg[k].sum : null));
    const base = vals[0];
    let rel = 0;
    for (const v of vals.slice(1)) {
      if (v === base) continue;
      rel = Math.max(rel, base == null || v == null ? Infinity
        : base === 0 ? Number.MAX_VALUE : Math.abs(v - base) / Math.abs(base));
    }
    if (rel > 0) rows.push({ k, vals, rel });
  }
  rows.sort((a, b) => b.rel - a.rel);
  return { rows, keys: keys.length };
}
