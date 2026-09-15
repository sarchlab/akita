import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

// Exercise the hook with a boundary-blocked endpoint and a controlled timer.
test("virtual-time polling leaves connections available for engine controls", async () => {
  const source = await readFile(new URL("../src/hooks/useEngineTime.ts", import.meta.url), "utf8");
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS },
  }).outputText;
  let tick, cleanup;
  const requests = [];
  const updates = [];
  const exports = {};
  vm.runInNewContext(compiled, {
    exports,
    require: () => ({
      useEffect: (effect) => { cleanup = effect(); },
      useState: () => [null, (value) => updates.push(value)],
    }),
    AbortController,
    window: { setInterval: (callback) => { tick = callback; return 1; }, clearInterval() {} },
    fetch: (_path, options) => new Promise((resolve) => requests.push({ resolve, ...options })),
  });
  exports.useEngineTime(500);
  for (let i = 0; i < 20; i++) tick();
  assert.equal(requests.length, 1, "a long event must not accumulate blocked time requests");
  requests[0].resolve({ ok: true, json: async () => ({ now: 42 }) });
  await new Promise(setImmediate);
  assert.deepEqual(updates, [42]);
  tick();
  assert.equal(requests.length, 2, "polling must resume once the boundary responds");
  cleanup();
  assert.equal(requests[1].signal.aborted, true);
  requests[1].resolve({ ok: true, json: async () => ({ now: 43 }) });
  await new Promise(setImmediate);
  assert.deepEqual(updates, [42], "unmounted views must not receive a late result");
});
