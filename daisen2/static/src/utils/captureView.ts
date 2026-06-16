import html2canvas from "html2canvas";

// Cap the captured image so the data URL stays small enough to send to the model.
const MAX_WIDTH = 1400;
const JPEG_QUALITY = 0.75;

function canvasToDataURL(canvas: HTMLCanvasElement): string {
  if (canvas.width <= MAX_WIDTH) return canvas.toDataURL("image/jpeg", JPEG_QUALITY);
  const scale = MAX_WIDTH / canvas.width;
  const scaled = document.createElement("canvas");
  scaled.width = Math.round(canvas.width * scale);
  scaled.height = Math.round(canvas.height * scale);
  const ctx = scaled.getContext("2d");
  if (!ctx) return canvas.toDataURL("image/jpeg", JPEG_QUALITY);
  ctx.drawImage(canvas, 0, 0, scaled.width, scaled.height);
  return scaled.toDataURL("image/jpeg", JPEG_QUALITY);
}

// Capture what the user is currently looking at — the main content area, which
// excludes the chat panel (a sibling of <main>).
export async function captureCurrentView(): Promise<string> {
  const target = (document.querySelector("main") as HTMLElement | null) ?? document.body;
  const canvas = await html2canvas(target, { backgroundColor: "#ffffff", logging: false });
  return canvasToDataURL(canvas);
}

// The Daisen view paths a daisen_view tool call may render (see App.tsx routes).
const ALLOWED_VIEW_PATHS = new Set(["/dashboard", "/component", "/task"]);

// toSafeDaisenUrl resolves a tool-supplied URL against our own origin and rejects
// anything that is not a same-origin Daisen view path. The url is model-controlled
// (a compromised/prompt-injected provider), and it is assigned to a same-origin
// iframe — so a `javascript:`/`data:`/cross-origin value could otherwise run in the
// Daisen page and read stored LLM settings/API keys.
export function toSafeDaisenUrl(raw: string): string {
  let resolved: URL;
  try {
    resolved = new URL(raw, window.location.origin);
  } catch {
    throw new Error(`invalid view url: ${raw}`);
  }
  if (resolved.origin !== window.location.origin) {
    throw new Error(`refusing to render a cross-origin view: ${raw}`);
  }
  if (!ALLOWED_VIEW_PATHS.has(resolved.pathname)) {
    throw new Error(`refusing to render a non-Daisen view path: ${resolved.pathname}`);
  }
  return resolved.pathname + resolved.search;
}

// Render a Daisen view off-screen (in an iframe) at the given URL, wait for it to
// finish (the Phase 1 render-ready signal), then capture it.
export async function captureUrl(url: string): Promise<string> {
  const safeUrl = toSafeDaisenUrl(url);
  const iframe = document.createElement("iframe");
  iframe.setAttribute("aria-hidden", "true");
  iframe.style.cssText = "position:fixed;left:-10000px;top:0;width:1280px;height:800px;border:0;";
  iframe.src = safeUrl;
  document.body.appendChild(iframe);
  try {
    await waitForViewReady(iframe, 20000);
    await new Promise((resolve) => setTimeout(resolve, 250)); // settle fonts/frames
    const doc = iframe.contentDocument;
    if (!doc) throw new Error("could not access the rendered view");
    const target = (doc.querySelector("main") as HTMLElement | null) ?? doc.body;
    const canvas = await html2canvas(target, {
      backgroundColor: "#ffffff",
      logging: false,
      width: 1280,
      windowWidth: 1280,
      windowHeight: 800,
    });
    return canvasToDataURL(canvas);
  } finally {
    iframe.remove();
  }
}

function waitForViewReady(iframe: HTMLIFrameElement, timeoutMs: number): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const start = Date.now();
    const timer = window.setInterval(() => {
      let ready = false;
      try {
        ready =
          (iframe.contentWindow as unknown as { __daisenViewReady?: boolean })?.__daisenViewReady === true;
      } catch {
        // Cross-origin briefly during load — keep waiting.
      }
      if (ready) {
        window.clearInterval(timer);
        resolve();
      } else if (Date.now() - start > timeoutMs) {
        window.clearInterval(timer);
        reject(new Error("the view did not finish rendering in time"));
      }
    }, 150);
  });
}
