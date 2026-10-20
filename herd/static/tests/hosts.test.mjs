// Adding a server: the entry built from the proposal and the form, and the
// result name of a hand-started simulation's launcher.
import assert from "node:assert/strict";
import { test } from "node:test";
import { entryForm, entryFrom, launcherRun } from "../src/lib/hosts.ts";

const proposed = { name: "server1", type: "ssh", address: "me@server1", queue: { slots: 32, mem_reserve_gb: 25 } };

test("the proposal round-trips through the form", () => {
  const f = entryForm(proposed);
  assert.deepEqual(f, { name: "server1", root: "~/exp", slots: "32", reserve: "25" });
  assert.deepEqual(entryFrom(proposed, f), proposed); // ~/exp is the default: no root
});

test("edits: name, root, queue", () => {
  const e = entryFrom(proposed, { name: " s1 ", root: "~/work", slots: "8", reserve: "" });
  assert.deepEqual(e, { name: "s1", type: "ssh", address: "me@server1", root: "~/work", queue: { slots: 8 } });
  assert.equal(entryFrom(proposed, { name: "s1", root: "", slots: "", reserve: "4" }).queue, undefined);
  assert.equal(entryFrom(proposed, { name: "s1", root: "", slots: "x", reserve: "4" }).queue, undefined);
  const dir = entryFrom({ ...proposed, akita_dir: "~/tools" }, entryForm(proposed));
  assert.equal(dir.akita_dir, "~/tools");
});

test("the result name a launcher mentions", () => {
  assert.equal(launcherRun("bash scripts/run.sh 1019_diag_x_base_a41f7e2 a41f7e2 -benchmark fir"), "1019_diag_x_base_a41f7e2");
  assert.equal(launcherRun("python3 sweep.py --out results/x 1019_a_b"), "1019_a_b");
  assert.equal(launcherRun("bash run.sh"), "");
  assert.equal(launcherRun(undefined), "");
});
