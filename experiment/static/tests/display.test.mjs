// Display helpers: MM-DD dates from the naming config, time-like property values,
// and the stable order behind compare_by colours.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { fmtTime, fmtTimeLike } from "../src/lib/format.ts";
import { buildFacets, compareBase, compareValues, mmdd, namingOf, valueLabel } from "../src/lib/naming.ts";

const here = dirname(fileURLToPath(import.meta.url));
const status = JSON.parse(readFileSync(join(here, "../fixtures/status.json"), "utf8"));
const naming = namingOf(status.projects, "demo");

test("four-digit date fields show as MM-DD only when valid", () => {
  assert.equal(valueLabel(naming, "date", "1015"), "10-15");
  assert.equal(valueLabel(naming, "date", "1399"), "1399"); // not a month/day
  assert.equal(valueLabel(naming, "date", "0230"), "0230");
  assert.equal(valueLabel(naming, "version", "1015"), "1015"); // other pattern
  assert.equal(valueLabel(naming, "category", "abl"), "ablation");
  assert.equal(mmdd({ name: "d", pattern: "\\d{6}" }, "101500"), null);
  assert.equal(mmdd({ name: "d", pattern: "[0-9]{4}" }, "0229"), "02-29");
  const date = buildFacets(status.runs, status.projects, {}).find((f) => f.id === "field:date");
  assert.deepEqual(date.values.map((v) => v.label), ["09-08", "09-07", "09-06"]); // newest first
  assert.equal(date.codes, false);
});

test("time-like property values", () => {
  const t = Date.parse("2026-10-15T15:00:16Z") / 1000;
  assert.equal(fmtTimeLike("2026-10-15T15:00:16+00:00"), fmtTime(t));
  assert.equal(fmtTimeLike("2026-10-15T15:00:16.259270Z"), fmtTime(t + 0.259));
  assert.equal(fmtTimeLike("2026-10-15 15:00:16"), fmtTime(Date.parse("2026-10-15T15:00:16") / 1000));
  assert.equal(fmtTimeLike(String(t)), fmtTime(t));
  for (const v of ["3e9a41c", "65", "base", "1234567", "2026-10-15", "12345678901"]) assert.equal(fmtTimeLike(v), null, v);
});

test("compare_by colours: one sorted list of base values", () => {
  const values = compareValues(status.runs, status.projects);
  assert.deepEqual(values, [...values].sort());
  assert.ok(values.includes("base") && values.includes("wide"));
  assert.ok(values.every((v) => !v.includes("+")));
  const r = { ...status.runs.find((x) => x.fields?.config === "base"), fields: { ...status.runs[0].fields, config: "base+mshr128" } };
  assert.equal(compareBase(r, status.projects), "base");
  assert.equal(compareBase({ ...r, fields: null }, status.projects), null);
});
