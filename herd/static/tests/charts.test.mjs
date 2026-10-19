// Chart numbers: ratio, GMEAN rule, baseline sources and sensitivity sweeps of
// the generic chart model (CampaignCells + plot specs), checked against
// independent computations and against the original monitor's own chart code
// (extracted from exp_web.py when it is installed; EXP_WEB_PY overrides the
// path), fed with the same campaigns in its own format.
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  buildCharts, chartCSV, defaultPlot, geomean, pairOf, prepareCampaign, selectionSpecs,
} from "../src/lib/charts/model.ts";
import { chartSVG, THEMES } from "../src/lib/charts/svg.ts";
import { setLocale } from "../src/lib/i18n.ts";

const here = dirname(fileURLToPath(import.meta.url));
const load = (n) => JSON.parse(readFileSync(join(here, "../fixtures", n), "utf8"));
// The same campaigns as read by a project's herd-cells/1 adapter (cells) and by
// the old monitor's reader (legacy). Names, hosts and numbers are anonymized:
// metric values are scaled by arbitrary factors, so they are not real results.
const cells = load("cells.json");
const legacy = load("legacy_campaigns.json");
const SPECS = load("status.json").projects.demo.plots;
const SPEEDUP = SPECS.find((s) => s.name === "speedup");

const K = {
  wgBase: "server2|demo|0908_abl_wg32k_base_3e9a41c",
  wgWide: "server1|demo|0908_abl_wg32k_wide_3e9a41c",
  spBase: "server2|demo|0908_abl_spmv-wg1536_base_3e9a41c",
  spWide: "server1|demo|0908_abl_spmv-wg1536_wide_3e9a41c",
  fullFast: "server2|demo|0906_abl_full_fast_7c25e90",
  fullWide: "server1|demo|0906_abl_full_wide_7c25e90",
  checkFast: "server2|demo|0906_check_full_fast_7c25e90",
  checkWide: "server1|demo|0906_check_full_wide_7c25e90",
  apps: "server2|demo|0908_apps_wg8k_base_3e9a41c",
  repro: "server2|demo|0908_repro_wg32k_base_3e9a41c",
  attr: "server1|demo|0907_abl_attr_fast_9b0c3d6",
};

setLocale("zh", false);

function ctxOf(docs, plot, specs = SPECS) {
  return {
    plot: { ...defaultPlot(), ...plot },
    data: (k) => (docs[k] ? prepareCampaign(docs[k], k) : null),
    specs, statusKind: () => null,
  };
}

const cellOf = (doc, w, mode, variant = "") =>
  doc.cells.find((c) => c.key.workload === w && c.key.variant === variant && c.key.config === mode);

// Independent reference: base / cfg total_time per workload when both ok with equal WGs.
function refGmean(doc, mode) {
  const s = [];
  for (const w of doc.order.workload) {
    const a = cellOf(doc, w, mode), b = cellOf(doc, w, "baseline");
    if (a?.status === "ok" && b?.status === "ok" && a.metrics.wgs === b.metrics.wgs) s.push(b.metrics.total_time / a.metrics.total_time);
  }
  return { n: s.length, g: Math.exp(s.reduce((x, v) => x + Math.log(v), 0) / s.length) };
}

test("geomean and pairOf", () => {
  const ok = (t, wgs) => ({ key: {}, status: "ok", metrics: { total_time: t, wgs } });
  assert.equal(geomean([]), null);
  assert.ok(Math.abs(geomean([2, 8]) - 4) < 1e-12);
  assert.equal(pairOf(SPEEDUP, null, null).flag, "missing");
  assert.equal(pairOf(SPEEDUP, { key: {}, status: "running", metrics: {} }, null).flag, "running");
  assert.equal(pairOf(SPEEDUP, ok(1, 1), { key: {}, status: "failed", metrics: {} }).flag, "no-baseline");
  assert.equal(pairOf(SPEEDUP, ok(1, 1), ok(2, 2)).flag, "mismatch");
  assert.equal(pairOf(SPEEDUP, ok(1, 2), ok(3, 2)).s, 3);
  assert.equal(pairOf({ ...SPEEDUP, lower_is_better: false }, ok(1, 2), ok(4, 2)).s, 0.25);
  assert.equal(pairOf(SPEEDUP, { key: {}, status: "ok", metrics: { wgs: 1 } }, ok(1, 1)).flag, "bad");
});

test("ablation: GMEAN over the workloads every configuration has", () => {
  const sel = [K.wgBase, K.spBase, K.wgWide, K.spWide];
  assert.equal(buildCharts(ctxOf(cells, { sel })).kind, "compare"); // automatic with >= 2 plain campaigns
  const abl = buildCharts(ctxOf(cells, { sel, kind: "ablation" }));
  assert.equal(abl.spec.name, "speedup");
  assert.equal(abl.models.length, 4);
  const byId = Object.fromEntries(abl.models.map((m) => [m.id, m]));
  const wg = byId["ablation_0908_abl_wg32k_base_3e9a41c"];
  // kmeans is still running on base: 12 of 13 workloads count.
  assert.equal(wg.common.length, 12);
  assert.ok(!wg.common.includes("kmeans"));
  const running = cells[K.wgBase].cells.filter((c) => c.status === "running" && c.key.config !== "baseline").length;
  assert.equal(wg.flagCounts.running, running);
  assert.deepEqual(wg.series.map((s) => s.label), ["Opt A", "Opt B", "Opt C", "Full"]);
  assert.equal(wg.labels.bitonicsort, "BT");
  for (const mode of ["optA", "optB", "optC", "optFull"]) {
    const ref = refGmean(cells[K.wgBase], mode);
    assert.ok(Math.abs(wg.series.find((s) => s.mode === mode).gmean - ref.g) < 1e-12, mode);
  }
  const hp = byId["ablation_0908_abl_wg32k_wide_3e9a41c"];
  assert.equal(hp.common.length, 13);
  assert.ok(Math.abs(hp.series.find((s) => s.mode === "optFull").gmean - refGmean(cells[K.wgWide], "optFull").g) < 1e-12);
  assert.equal(byId["ablation_0908_abl_spmv-wg1536_base_3e9a41c"].common.length, 1);
});

test("compare: GMEAN only over workloads both campaigns have; hide re-computes", () => {
  const [m] = buildCharts(ctxOf(cells, { sel: [K.wgBase, K.wgWide], kind: "compare" })).models;
  assert.equal(m.id, "compare_optFull");
  assert.equal(m.common.length, 12);
  assert.deepEqual(m.series.map((s) => s.label), ["A base", "B wide"]);
  const hidden = buildCharts(ctxOf(cells, { sel: [K.wgBase, K.wgWide], kind: "compare", hide: ["camp:" + K.wgBase] })).models[0];
  assert.equal(hidden.visible.length, 1);
  assert.equal(hidden.common.length, 13);
  assert.ok(Math.abs(hidden.visible[0].gmean - refGmean(cells[K.wgWide], "optFull").g) < 1e-12);
  const disjoint = buildCharts(ctxOf(cells, { sel: [K.wgBase, K.spBase], kind: "compare" })).models[0];
  assert.equal(disjoint.common.length, 0);
  assert.ok(disjoint.series.every((s) => s.gmean == null));
});

test("baseline borrowed from another campaign (check campaigns ran only optFull)", () => {
  const m = buildCharts(ctxOf(cells, { sel: [K.checkFast, K.fullWide, K.fullFast], kind: "ablation" })).models[0];
  assert.equal(m.series[0].baseSrc, K.fullFast); // same host first
  assert.equal(m.series.length, 1);
  assert.ok(m.sub.some((s) => s.includes("baseline 来自 C")));
});

test("generic specs: other metric, higher is better, no summary, derived default", () => {
  const wgs = { ...SPEEDUP, name: "wgratio", label: "WG ratio", metric: "wgs", lower_is_better: false, match: [], summary: "" };
  const m = buildCharts(ctxOf(cells, { sel: [K.wgWide], kind: "ablation" }, [wgs])).models[0];
  assert.ok(m.series.every((s) => s.gmean == null));
  const p = m.series[0].pairs.aes;
  assert.equal(p.s, p.cell.metrics.wgs / p.base.metrics.wgs);
  const csv = chartCSV(m);
  assert.ok(!csv.includes("GMEAN"));
  assert.match(csv.split("\n")[0], /,wgs,wgratio$/);
  const d = selectionSpecs([K.wgWide], (k) => prepareCampaign(cells[k], k), () => null);
  assert.equal(d.derived, true);
  assert.deepEqual([d.specs[0].x, d.specs[0].series, d.specs[0].baseline, d.specs[0].metric], ["workload", "config", "baseline", "total_time"]);
});

test("CSV and SVG render; English strings", () => {
  const m = buildCharts(ctxOf(cells, { sel: [K.wgBase], kind: "ablation", cap: 2 })).models[0];
  const csv = chartCSV(m);
  assert.equal(csv.split("\n")[0], "chart,series,variant,point_value,workload,label,config,flag,note,baseline_source,baseline_total_time,baseline_wgs,source,total_time,wgs,speedup");
  assert.equal(csv.trim().split("\n").length, 1 + 4 * 14);
  const tips = [];
  const svg = chartSVG(m, THEMES.light, 900, 0, tips, false);
  assert.match(svg, /^<svg /);
  assert.equal(tips.length, 4 * 14);
  assert.ok(chartSVG(m, THEMES.dark, 400, 0, [], true).includes("GMEAN"));
  setLocale("en", false);
  try {
    const e = buildCharts(ctxOf(cells, { sel: [K.wgBase], kind: "ablation" })).models[0];
    assert.equal(e.title, "Ablation: A 0908_abl_wg32k_base_3e9a41c");
    assert.match(chartSVG(e, THEMES.light, 900, 0, [], false), />Speedup \(×\)</);
  } finally {
    setLocale("zh", false);
  }
});

// ---------- parity with the old page ----------

const OLD = process.env.EXP_WEB_PY || join(homedir(), ".local/share/exp/exp_web.py");

function oldCharts() {
  if (!existsSync(OLD)) return null;
  const src = readFileSync(OLD, "utf8");
  const a = src.indexOf("// ---------- charts: campaign speedups");
  const b = src.indexOf("// ---------- SVG ----------");
  const c = src.indexOf("function plotState()"), e = src.indexOf("function plotChipsHTML()");
  if (a < 0 || b < 0 || c < 0 || e < 0) return null;
  const body = src.slice(a, b) + src.slice(c, e);
  return (ui, data) => {
    const f = new Function("ui", "last", "render", "renderCharts", "fetch", "alert", "window",
      body + "\nreturn {camps, ablationModels, compareModel, sensitivitySetup, sensitivityModels, plotState};");
    const o = f(ui, { hosts: {} }, () => {}, () => {}, () => Promise.reject(new Error("no")), () => {}, {});
    for (const [k, d] of Object.entries(data)) o.camps[k] = { data: d, at: 0 };
    return o;
  };
}

const FLAG_NAME = { "wg-mismatch": "mismatch" };
const newFlag = (f) => FLAG_NAME[f] ?? f;

function sameModel(nm, om, strict) {
  assert.equal(nm.id, om.id);
  assert.deepEqual(nm.workloads, om.workloads);
  assert.deepEqual(nm.common, om.common);
  assert.deepEqual(nm.flagCounts, Object.fromEntries(Object.entries(om.flagCounts).map(([f, n]) => [newFlag(f), n])));
  assert.deepEqual(nm.labels, om.labels);
  assert.deepEqual(nm.series.map((s) => [s.hideKey, s.baseSrc, s.mode, s.variant, s.gmean, s.pointValue ?? null, !!s.ref]),
    om.series.map((s) => [s.hideKey, s.baseSrc, s.mode, s.variant, s.gmean, s.pointValue ?? null, !!s.ref]));
  for (const [i, s] of nm.series.entries()) {
    for (const w of nm.workloads) {
      assert.equal(s.pairs[w].flag, newFlag(om.series[i].pairs[w].flag), `${nm.id} ${s.key} ${w}`);
      assert.equal(s.pairs[w].s, om.series[i].pairs[w].s, `${nm.id} ${s.key} ${w}`);
    }
  }
  if (om.line) {
    assert.deepEqual(nm.line.points.map((p) => [p.tick, p.v, p.hidden, !!p.ref]), om.line.points.map((p) => [p.tick, p.v, p.hidden, !!p.ref]));
    assert.equal(nm.line.n, om.line.n);
  }
  // Texts that do not name the cells' labels are the old page's, word for word.
  if (strict) {
    assert.deepEqual(nm.diffs ?? null, om.diffs ?? null);
    assert.deepEqual(nm.warn, om.warn);
  }
}

let checked = 0;
function runBoth(oldData, newData, plot, strict = true) {
  const mk = oldCharts();
  const ui = { plot: { ...defaultPlot(), config: "optFull", ...structuredClone(plot) } };
  const old = mk(ui, oldData);
  const { kind } = old.plotState();
  let o;
  if (kind === "sensitivity") {
    const st = old.sensitivitySetup();
    o = st.d ? old.sensitivityModels(st) : st;
  } else o = kind === "compare" ? old.compareModel() : old.ablationModels();
  const n = buildCharts(ctxOf(newData, plot));
  assert.equal(n.kind, kind);
  assert.equal(n.models.length, (o.models || []).length);
  n.models.forEach((m, i) => sameModel(m, o.models[i], strict));
  assert.deepEqual(n.notes.length, (o.notes || []).length);
  checked += n.models.reduce((a, m) => a + m.series.length * m.workloads.length, 0);
  return n;
}

// The old reader's campaign format as CampaignCells, for synthetic campaigns.
const POINT_FLAGS = ["-l2-cache-size-mb="];
function legacyToCells(d) {
  const out = [];
  for (const [k, c] of Object.entries(d.cells)) {
    const [workload, variant, config] = k.split("|");
    const metrics = c.status === "ok" ? { total_time: c.t, wgs: c.wgs } : {};
    const cmd = workload === d.cmd_bench ? d.commands?.[`${variant}|${config}`] : null;
    out.push({ key: { workload, variant, config }, status: c.status, reason: c.reason, metrics, ...(cmd ? { command: cmd } : {}) });
  }
  const sweep = d.sens ? {
    dimensions: d.sens.dims.map((dim) => {
      const unit = dim.points[0]?.legend.slice(dim.points[0].tick.length + 1) ?? "";
      return {
        name: dim.name, label: dim.title, unit,
        points: [...dim.points.map((p) => ({ value: p.value, label: p.tick, cell: { variant: p.name }, reference: false })),
          { value: dim.ref_value, label: dim.ref_tick, cell: d.sens.reference ? { variant: d.sens.reference } : null, reference: true }],
      };
    }),
  } : null;
  return {
    schema: "herd-cells/1", dimensions: ["workload", "variant", "config"],
    labels: { workload: d.labels }, order: { workload: d.workloads, variant: d.variants, config: d.modes },
    cells: out, sweep, total: d.total, warnings: d.warnings, binary: d.binary,
    meta: { ...d.meta, kind: d.kind, ...(d.sens ? { sweep_flags: POINT_FLAGS } : {}) },
    host: d.host, project: d.project, name: d.name,
  };
}

// A synthetic sensitivity campaign (no real one exists yet): two dims, a
// reference point, a failed cell and a WG mismatch; plus one without its own
// reference.
function sensCampaign(name, host, withRef) {
  const workloads = ["aes", "fir", "kmeans"];
  const points = [["cap16", 16], ["cap32", 32], ["cap128", 128]];
  const variants = points.map((p) => p[0]).concat(withRef ? ["cap64"] : []);
  const cs = {}, commands = {};
  let t = 1;
  for (const v of variants) {
    for (const w of workloads) {
      for (const mode of ["baseline", "optFull"]) {
        t += 0.37;
        const c = { status: "ok", reason: "", t: (mode === "baseline" ? 2 : 1.5) * (1 + (t % 1)), wgs: 100, config: mode };
        if (v === "cap32" && w === "fir" && mode === "optFull") Object.assign(c, { status: "failed", reason: "exit 1", t: null, wgs: null });
        if (v === "cap128" && w === "kmeans" && mode === "optFull") c.wgs = 90;
        cs[`${w}|${v}|${mode}`] = c;
        commands[`${v}|${mode}`] = ["-benchmark=aes", "-timing", `-l2-cache-size-mb=${v.slice(3)}`, mode === "optFull" ? "-x=1" : "-x=0"];
      }
    }
  }
  return {
    name, host, project: "p", kind: "sensitivity", workloads, labels: { aes: "AES", fir: "FIR", kmeans: "KM" },
    variants, modes: ["baseline", "optFull"], cells: cs, commands, binary: "abc", cmd_bench: "aes",
    counts: { ok: 1, failed: 1, running: 0, missing: 0, bad: 0 }, total: 24, finished: true, warnings: [], meta: {},
    sens: {
      reference: withRef ? "cap64" : null,
      dims: [
        { name: "cap", title: "容量", xlabel: "项数", ref_value: 64, ref_tick: "64", ref_legend: "64 项",
          points: points.map(([n, v]) => ({ name: n, value: v, tick: String(v), legend: `${v} 项` })) },
        { name: "cap2", title: "容量 2", xlabel: "项数", ref_value: 32, ref_tick: "32", ref_legend: "32 项",
          points: points.map(([n, v]) => ({ name: n, value: v, tick: String(v), legend: `${v} 项` })) },
      ],
    },
  };
}

const parity = oldCharts() ? test : test.skip;

parity("parity with the old page: real campaigns (ablation, compare, hide, cap, base override)", () => {
  const sel = [K.wgBase, K.spBase, K.wgWide, K.spWide];
  runBoth(legacy, cells, { sel, kind: "ablation" });
  runBoth(legacy, cells, { sel, kind: "compare" });
  runBoth(legacy, cells, { sel, kind: "compare", config: "optA", hide: ["camp:" + K.spWide] });
  runBoth(legacy, cells, { sel: [K.wgBase, K.wgWide], kind: "ablation", base: { [K.wgBase]: K.wgWide }, hide: ["cfg:optC"], cap: 2 });
  runBoth(legacy, cells, { sel: [K.wgWide, K.wgBase], kind: "" });
  // failed / missing / running cells, borrowed baselines, many operators, few configurations
  runBoth(legacy, cells, { sel: [K.fullFast, K.fullWide, K.checkFast, K.checkWide], kind: "ablation" });
  runBoth(legacy, cells, { sel: [K.fullFast, K.fullWide, K.checkFast, K.checkWide], kind: "compare" });
  runBoth(legacy, cells, { sel: [K.checkWide, K.checkFast, K.fullWide], kind: "ablation", base: { [K.checkWide]: K.fullWide } });
  runBoth(legacy, cells, { sel: [K.apps, K.repro, K.attr], kind: "ablation" });
  runBoth(legacy, cells, { sel: [K.apps, K.repro, K.attr, K.wgBase], kind: "compare", config: "optB" });
  runBoth(legacy, cells, { sel: [K.repro, K.wgBase], kind: "compare" });
});

parity("parity with the old page: sensitivity options", () => {
  const sA = "h1|p|s_ref", sB = "h1|p|s_noref";
  const oldData = { ...legacy, [sA]: sensCampaign("s_ref", "h1", true), [sB]: sensCampaign("s_noref", "h1", false) };
  const newData = { ...cells, [sA]: legacyToCells(oldData[sA]), [sB]: legacyToCells(oldData[sB]) };
  const n = runBoth(oldData, newData, { sel: [sA] });
  assert.equal(n.kind, "sensitivity");
  assert.equal(n.models.length, 2);
  assert.ok(n.models[0].series.some((s) => s.ref)); // cap64 reference point added
  assert.deepEqual(n.models[0].series.map((s) => s.label), ["16 项", "32 项", "64 项（参考）", "128 项"]);
  const bars = chartSVG(n.models[0], THEMES.light, 700, 0, [], false);
  const line = chartSVG(n.models[0].line, THEMES.dark, 300, 1, [], false);
  assert.match(bars, />AES</);
  assert.match(line, />64\*</);
  assert.match(line, /GMEAN Speedup（×）/);
  runBoth(oldData, newData, { sel: [sA], ref: "none" });
  runBoth(oldData, newData, { sel: [sA], norm: "fixed" });
  runBoth(oldData, newData, { sel: [sA], hide: ["pt:cap32"] });
  runBoth(oldData, newData, { sel: [sB, sA], sens: sB });
  runBoth(oldData, newData, { sel: [sB, sA], sens: sB, norm: "fixed", fixed: sA });
  runBoth(oldData, newData, { sel: [sB, K.wgBase], ref: K.wgBase, norm: "fixed" }, false);
  runBoth(oldData, newData, { sel: [sB, K.wgBase], kind: "ablation" });
  assert.ok(checked > 800, String(checked));
});
