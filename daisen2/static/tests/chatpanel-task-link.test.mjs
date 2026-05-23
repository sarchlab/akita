import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const TASK_ID = "d240btg3fvio1hp2d3eg";
const LOCALHOST_TASK_URL = ["http:", "", "localhost:5173", "task"].join("/") + `?id=${TASK_ID}`;
const SAME_ORIGIN_TASK_PATH = `/task?id=${TASK_ID}`;

const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

test("task navigation uses same-origin task links", async () => {
  const source = await readFile(new URL("../src/pages/TaskChartPage.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(source, new RegExp(escapeRegExp(LOCALHOST_TASK_URL)));
  assert.match(source, /setSearchParams\(\{\s*id\s*\}\)/);
});
