// The dictionary: both languages for every key, with the same parameters;
// every t("...") key used in the sources exists.
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { MESSAGES, setLocale, t } from "../src/lib/i18n.ts";

const here = dirname(fileURLToPath(import.meta.url));
const params = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test("every key has English and Chinese with the same parameters", () => {
  for (const [k, [en, zh]] of Object.entries(MESSAGES)) {
    assert.ok(en && zh, k);
    assert.deepEqual(params(en), params(zh), k);
  }
});

test("keys used in the sources exist; t() fills parameters", () => {
  const files = [];
  const walk = (d) => { for (const f of readdirSync(d)) { const p = join(d, f); if (statSync(p).isDirectory()) walk(p); else if (/\.tsx?$/.test(f)) files.push(p); } };
  walk(join(here, "../src"));
  for (const f of files) {
    for (const m of readFileSync(f, "utf8").matchAll(/\bt\("([\w.-]+)"/g)) assert.ok(m[1] in MESSAGES, `${f}: ${m[1]}`);
  }
  setLocale("en", false);
  assert.equal(t("charts.selected", { n: 2, max: 4 }), "2 campaigns selected (at most 4)");
  setLocale("zh", false);
  assert.equal(t("charts.selected", { n: 2, max: 4 }), "已选 2 个 campaign（最多 4 个）");
  setLocale("en", false);
});
