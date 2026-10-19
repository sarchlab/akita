// Generic naming: facets, variant base, grouping by group_by, peers.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  buildFacets, filterRuns, groupKey, groupPeers, groupRuns, namingOf, otherFields, variantBase,
} from "../src/lib/naming.ts";
import { setLocale } from "../src/lib/i18n.ts";

const here = dirname(fileURLToPath(import.meta.url));
const status = JSON.parse(readFileSync(join(here, "../fixtures/status.json"), "utf8"));
const P = status.projects;
const naming = namingOf(P, "demo");

test("variant base uses the field's variant_separator", () => {
  assert.equal(variantBase(naming, "config", "base+mshr128+x"), "base");
  assert.equal(variantBase(naming, "config", "base"), "base");
  assert.equal(variantBase(naming, "item", "a+b"), "a+b"); // no separator on this field
  assert.equal(variantBase(null, "config", "base+x"), "base+x");
});

test("facets come from the naming config", () => {
  const facets = buildFacets(status.runs, P, {});
  const ids = facets.map((f) => f.id);
  assert.ok(ids.includes("field:category"));
  assert.ok(ids.includes("field:config"));
  assert.ok(ids.includes("host") && ids.includes("kind"));
  assert.ok(!ids.includes("field:item"), "item has many distinct values and no vocabulary");
  const cat = facets.find((f) => f.id === "field:category");
  assert.equal(cat.label, "类别");
  assert.equal(cat.values.find((v) => v.value === "abl").label, "ablation");
  // vocabulary order
  assert.deepEqual(cat.values.map((v) => v.value), Object.keys(naming.fields[1].values).filter((v) => cat.values.some((x) => x.value === v)));
  const total = cat.values.reduce((a, v) => a + v.count, 0);
  assert.equal(total, status.runs.filter((r) => r.fields).length);
  // config facet groups variants under their base
  const cfg = facets.find((f) => f.id === "field:config");
  assert.ok(cfg.values.every((v) => !v.value.includes("+")));
});

test("facet counts respect the other selections", () => {
  const sel = { "field:category": ["abl"] };
  const facets = buildFacets(status.runs, P, sel);
  const host = facets.find((f) => f.id === "host");
  const abl = status.runs.filter((r) => r.fields?.category === "abl");
  assert.equal(host.values.reduce((a, v) => a + v.count, 0), abl.length);
  // the category facet itself still counts every category
  const cat = facets.find((f) => f.id === "field:category");
  assert.ok(cat.values.filter((v) => v.count > 0).length > 1);
  assert.equal(filterRuns(status.runs, sel, P).length, abl.length);
  const two = filterRuns(status.runs, { "field:category": ["abl"], kind: ["campaign"] }, P);
  assert.ok(two.length > 0 && two.every((r) => r.kind === "campaign" && r.fields.category === "abl"));
});

test("grouping by group_by, unparsed runs and the item limit", () => {
  const runs = status.runs.concat([{ ...status.runs[0], id: "x", name: "weird-name", fields: null, mtime: 1 }]);
  const { groups, total } = groupRuns(runs, P);
  const g = groups.find((x) => x.label === "ablation · wg32k");
  assert.ok(g, groups.map((x) => x.label).join(","));
  assert.ok(g.runs.every((r) => r.fields.category === "abl" && r.fields.item === "wg32k"));
  const un = groups.find((x) => !x.parsed);
  assert.equal(un.label, "unparsed");
  setLocale("zh", false);
  assert.equal(groupRuns(runs, P).groups.find((x) => !x.parsed).label, "未解析");
  setLocale("en", false);
  assert.deepEqual(un.runs.map((r) => r.name), ["weird-name"]);
  const limited = groupRuns(runs, P, 3);
  assert.equal(limited.groups.length, 3);
  assert.equal(limited.hidden, total - 3);
  // projects null (live server before exp.json is deployed): every run unparsed, one item each
  const live = groupRuns(status.runs.map((r) => ({ ...r, fields: null })), null, 50);
  assert.equal(live.groups.length, 1);
  assert.equal(live.groups[0].runs.length, 50);
});

test("same-group campaigns (同类对比), any host, newest first", () => {
  const r = status.runs.find((x) => x.name === "0908_abl_wg32k_base_3e9a41c");
  const key = groupKey(r, naming);
  const peers = groupPeers(status.runs, P, key, false);
  assert.ok(peers.length >= 2 && peers.length <= 4);
  assert.ok(new Set(peers.map((p) => p.host)).size === 2);
  assert.ok(peers.every((p) => p.kind !== "single"));
  for (let i = 1; i < peers.length; i++) assert.ok(peers[i - 1].mtime >= peers[i].mtime);
  assert.deepEqual(otherFields(naming, r.fields).map((f) => f.name), ["date", "version"]);
});
