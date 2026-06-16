// Render-ready signal: a single source of truth for "the current view has
// finished loading its data and painting", exposed so an off-screen / headless
// capture (Phase 5) can await it. See daisenbot-agent-plan.md (Phase 1,
// Workstream D). Pure tracker — no React, no framework — so it is directly
// testable with `node --test`; the React glue lives in hooks/useRenderReady.ts.
//
// It exposes the signal two ways (a boolean for scripts, an attribute for CSS
// selectors / CDP waitForSelector):
//   window.__daisenViewReady : boolean
//   <html data-daisen-ready="ok|error">   (attribute absent while not ready)
//
// Data hooks register each in-flight fetch via beginRenderWork(); when the count
// returns to zero we wait one frame + document.fonts.ready (so the SVG is
// committed and text is measured against loaded fonts) and then flip ready true.
// A view that ends up empty or errored still settles, so a capture never hangs.

let pending = 0;
let batchErrored = false;

/** @type {(cb: () => void) => void} */
let scheduler = defaultScheduler;

function defaultScheduler(cb) {
  const w = /** @type {any} */ (globalThis).window;
  const raf =
    w && typeof w.requestAnimationFrame === "function"
      ? w.requestAnimationFrame.bind(w)
      : (fn) => setTimeout(fn, 0);
  const fonts = globalThis.document && globalThis.document.fonts;
  raf(() => {
    if (fonts && fonts.ready && typeof fonts.ready.then === "function") {
      fonts.ready.then(cb, cb);
    } else {
      cb();
    }
  });
}

function setReady(ready, status) {
  const w = /** @type {any} */ (globalThis).window;
  if (w) w.__daisenViewReady = ready;
  const root = globalThis.document && globalThis.document.documentElement;
  if (!root) return;
  if (ready) root.setAttribute("data-daisen-ready", status || "ok");
  else root.removeAttribute("data-daisen-ready");
}

function settleIfIdle() {
  if (pending !== 0) return;
  const status = batchErrored ? "error" : "ok";
  batchErrored = false;
  scheduler(() => {
    // Re-check: new work may have started before this frame fired.
    if (pending === 0) setReady(true, status);
  });
}

/**
 * Register one unit of in-flight render work. Returns an `end` callback to invoke
 * when it settles; pass `{ errored: true }` if the work failed.
 * @returns {(result?: { errored?: boolean }) => void}
 */
export function beginRenderWork() {
  pending += 1;
  if (pending === 1) setReady(false);
  let ended = false;
  return (result) => {
    if (ended) return;
    ended = true;
    if (result && result.errored) batchErrored = true;
    pending = Math.max(0, pending - 1);
    settleIfIdle();
  };
}

/**
 * Mark that a navigation just started: the previous view's "ready" must not carry
 * over. Sets the signal false immediately; if no fetch follows, it settles back
 * to ready on the next frame.
 */
export function markNavigation() {
  setReady(false);
  settleIfIdle();
}

/** @returns {{ pending: number, errored: boolean }} current tracker state. */
export function getRenderState() {
  return { pending, errored: batchErrored };
}

// Test seam: swap in a synchronous scheduler and reset state between cases.
export const __testHooks = {
  reset() {
    pending = 0;
    batchErrored = false;
    scheduler = defaultScheduler;
  },
  useSyncScheduler() {
    scheduler = (cb) => cb();
  },
};
