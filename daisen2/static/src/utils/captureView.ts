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

// Render a Daisen view off-screen (in an iframe) at the given URL, wait for it to
// finish (the Phase 1 render-ready signal), then capture it.
export async function captureUrl(url: string): Promise<string> {
  const iframe = document.createElement("iframe");
  iframe.setAttribute("aria-hidden", "true");
  iframe.style.cssText = "position:fixed;left:-10000px;top:0;width:1280px;height:800px;border:0;";
  iframe.src = url;
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
