import assert from "node:assert/strict";
import test from "node:test";

import {
  beginRenderWork,
  markNavigation,
  getRenderState,
  __testHooks,
} from "../src/utils/renderReady.mjs";

// Minimal DOM/window stub so the tracker can publish its signal under node.
function installDom() {
  const attrs = {};
  globalThis.window = {};
  globalThis.document = {
    documentElement: {
      setAttribute: (key, value) => {
        attrs[key] = value;
      },
      removeAttribute: (key) => {
        delete attrs[key];
      },
      getAttribute: (key) => (key in attrs ? attrs[key] : null),
    },
  };
  return attrs;
}

function setup() {
  const attrs = installDom();
  __testHooks.reset();
  __testHooks.useSyncScheduler(); // settle runs synchronously
  return attrs;
}

const ready = () => globalThis.window.__daisenViewReady === true;

test("a single unit of work flips ready false then true (ok)", () => {
  const attrs = setup();
  const end = beginRenderWork();
  assert.equal(ready(), false);
  assert.equal(attrs["data-daisen-ready"] ?? null, null);

  end();
  assert.equal(ready(), true);
  assert.equal(attrs["data-daisen-ready"], "ok");
});

test("ready only flips true once ALL concurrent work has ended", () => {
  setup();
  const end1 = beginRenderWork();
  const end2 = beginRenderWork();
  assert.equal(getRenderState().pending, 2);

  end1();
  assert.equal(ready(), false); // one still in flight

  end2();
  assert.equal(ready(), true);
  assert.equal(getRenderState().pending, 0);
});

test("an errored unit makes the settled status 'error'", () => {
  const attrs = setup();
  const end = beginRenderWork();
  end({ errored: true });
  assert.equal(ready(), true); // still settles — a capture never hangs
  assert.equal(attrs["data-daisen-ready"], "error");
});

test("error status does not leak into the next batch", () => {
  const attrs = setup();
  beginRenderWork()({ errored: true });
  assert.equal(attrs["data-daisen-ready"], "error");

  const end = beginRenderWork();
  assert.equal(ready(), false);
  end();
  assert.equal(attrs["data-daisen-ready"], "ok");
});

test("ending the same unit twice is a no-op", () => {
  setup();
  const end1 = beginRenderWork();
  const end2 = beginRenderWork();
  end1();
  end1(); // double-end must not under-count
  assert.equal(ready(), false);
  assert.equal(getRenderState().pending, 1);
  end2();
  assert.equal(ready(), true);
});

test("markNavigation clears ready, then settles back when idle", () => {
  const attrs = setup();
  // start from a ready state
  beginRenderWork()();
  assert.equal(ready(), true);

  markNavigation();
  // idle + sync scheduler -> settles straight back to ready ok
  assert.equal(ready(), true);
  assert.equal(attrs["data-daisen-ready"], "ok");
});

test("markNavigation stays not-ready while a load is in flight", () => {
  setup();
  const end = beginRenderWork();
  markNavigation();
  assert.equal(ready(), false);
  end();
  assert.equal(ready(), true);
});
