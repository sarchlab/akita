import { useCallback, useLayoutEffect, useMemo, useRef } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import DOMPurify from "dompurify";
import type { ChatMessage, UnitContent } from "../../types/chat";
import { renderChatMarkdown } from "../../utils/chatMarkdown.mjs";
import { canonicalViewUrl } from "../../utils/viewState.mjs";
import { captureUrl } from "../../utils/captureView";
import { cn } from "../../lib/utils";
import type { LightboxImage } from "./Lightbox";

// A 1x1 transparent PNG used as an evidence thumbnail's src while its view renders
// off-screen, so the browser shows a pulsing placeholder (via CSS) instead of a
// broken-image icon — e.g. on chat load, where the persisted message stored no image.
const LOADING_PLACEHOLDER =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

// Pull a friendly rationale + query out of a tool call's JSON arguments, falling
// back to the raw string when it isn't the shape we expect.
function parseToolArgs(args?: string): { reason?: string; query?: string } {
  if (!args) return {};
  try {
    const o = JSON.parse(args) as Record<string, unknown>;
    const reason = typeof o.reason === "string" ? o.reason : undefined;
    const sql = typeof o.sql === "string" ? o.sql : undefined;
    return { reason, query: sql ?? args };
  } catch {
    return { query: args };
  }
}

export default function MessageBubble({
  message,
  onEnlarge,
}: {
  message: ChatMessage;
  onEnlarge?: (image: LightboxImage) => void;
}) {
  const images = message.content.filter(
    (u): u is Extract<UnitContent, { type: "image_url" }> => u.type === "image_url",
  );
  const text = message.content
    .filter((u): u is Extract<UnitContent, { type: "text" }> => u.type === "text")
    .map((u) => u.text)
    .join("\n");

  // Model output is untrusted (any configured provider, including local/unknown
  // models), so sanitize the markdown-generated HTML before injecting it.
  // DOMPurify keeps the safe markup markdown-it emits while stripping scripts,
  // event handlers, and other injection vectors. ADD_ATTR keeps the data-view-url
  // marker our chatMarkdown image rule puts on inline view-evidence figures.
  const html = useMemo(
    () => DOMPurify.sanitize(renderChatMarkdown(text), { ADD_ATTR: ["data-view-url"] }),
    [text],
  );

  const steps = message.role === "assistant" ? message.steps : undefined;
  const toolCount = steps?.filter((s) => s.tool).length ?? 0;

  // Map a canonical Daisen view URL → the image the agent already captured for it
  // (its daisen_view steps), so inline citations of those views show their thumbnail
  // with no extra render.
  const renderedViews = useMemo(() => {
    const map = new Map<string, string>();
    for (const step of steps ?? []) {
      if (step.tool !== "daisen_view" || !step.image || !step.args) continue;
      try {
        const url = (JSON.parse(step.args) as { url?: string }).url;
        const canon = url ? canonicalViewUrl(url) : null;
        if (canon) map.set(canon, step.image);
      } catch {
        // Non-JSON args — skip; the view simply won't have a pre-captured image.
      }
    }
    return map;
  }, [steps]);

  // Thumbnails lazily captured for cited views the agent did not render itself,
  // memoized across re-renders so a streaming update doesn't re-capture them.
  const lazyImages = useRef(new Map<string, string>());
  const proseRef = useRef<HTMLDivElement>(null);

  // Resolve each inline view-evidence thumbnail's src from the captured image (or
  // lazy-capture it off-screen). This runs after EVERY commit (no deps) and re-asserts
  // the src, because the src lives on a node inside dangerouslySetInnerHTML — outside
  // React's control — so any re-render that re-applies the prose HTML (e.g. when the
  // lightbox opens) would otherwise drop it and the image would fall back to its alt.
  // A layout effect re-applies it before paint, so the thumbnail never flickers to broken.
  useLayoutEffect(() => {
    if (!html.includes("daisen-evidence")) return;
    const root = proseRef.current;
    if (!root) return;
    root.querySelectorAll<HTMLImageElement>("img.daisen-evidence[data-view-url]").forEach((img) => {
      const viewUrl = img.getAttribute("data-view-url") ?? "";
      if (!viewUrl) return;
      const ready = renderedViews.get(viewUrl) ?? lazyImages.current.get(viewUrl);
      if (ready) {
        if (img.getAttribute("src") !== ready) img.src = ready;
        img.classList.remove("daisen-evidence-loading");
        return;
      }
      if (img.dataset.capturing) return;
      img.dataset.capturing = "1";
      // Show a gentle pulsing placeholder (transparent src → no broken-image icon)
      // while the view renders off-screen.
      if (!img.getAttribute("src")) img.src = LOADING_PLACEHOLDER;
      img.classList.add("daisen-evidence-loading");
      captureUrl(viewUrl)
        .then((data) => {
          if (!data) return;
          lazyImages.current.set(viewUrl, data);
          img.src = data;
          img.classList.remove("daisen-evidence-failed");
        })
        .catch(() => {
          // The view could not be rendered — drop the thumbnail, keep the caption link.
          img.classList.add("daisen-evidence-failed");
        })
        .finally(() => {
          img.classList.remove("daisen-evidence-loading");
          delete img.dataset.capturing;
        });
    });
  });

  // Clicking a thumbnail (not its caption link) opens the lightbox.
  const handleProseClick = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      const img = (event.target as HTMLElement).closest(
        "img.daisen-evidence",
      ) as HTMLImageElement | null;
      // Not while it's still the loading placeholder (a transparent src is truthy).
      if (!img || !img.src || img.classList.contains("daisen-evidence-loading")) return;
      event.preventDefault();
      onEnlarge?.({ src: img.src, viewUrl: img.getAttribute("data-view-url") ?? "" });
    },
    [onEnlarge],
  );

  return (
    <div className={cn("flex", message.role === "user" ? "justify-end" : "justify-start")}>
      <div className="flex max-w-[92%] flex-col gap-1.5">
        {steps && steps.length > 0 && (
          <details className="rounded-xl border bg-background/60 px-2.5 py-1.5 text-xs">
            <summary className="cursor-pointer select-none text-muted-foreground">
              {toolCount > 0 ? `${toolCount} tool call${toolCount > 1 ? "s" : ""}` : "reasoning"}
            </summary>
            <div className="mt-1.5 space-y-2">
              {steps.map((step, i) => {
                if (step.thinking) {
                  return (
                    <div key={i} className="whitespace-pre-wrap break-words italic text-muted-foreground">
                      {step.thinking}
                    </div>
                  );
                }
                const { reason, query } = parseToolArgs(step.args);
                return (
                  <div key={i} className="space-y-0.5">
                    <div className="font-medium text-foreground">{step.tool}</div>
                    {reason && (
                      <div className="whitespace-pre-wrap break-words italic text-muted-foreground">{reason}</div>
                    )}
                    {query && (
                      <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded bg-muted px-1.5 py-1 font-mono text-[11px]">
                        {query}
                      </pre>
                    )}
                    {step.observation && (
                      <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[11px] text-muted-foreground">
                        {step.observation}
                      </pre>
                    )}
                    {step.image && (
                      <img src={step.image} alt="captured view" className="mt-1 max-h-56 rounded-md border" />
                    )}
                  </div>
                );
              })}
            </div>
          </details>
        )}

        {images.length > 0 && (
          <div className={cn("flex flex-wrap gap-1.5", message.role === "user" ? "justify-end" : "justify-start")}>
            {images.map((img, i) => (
              <img
                key={i}
                src={img.image_url.url}
                alt="attachment sent to the model"
                className="max-h-44 rounded-lg border object-contain"
              />
            ))}
          </div>
        )}

        {text && (
          <div
            ref={proseRef}
            onClick={handleProseClick}
            className={cn(
              "chat-markdown rounded-2xl px-3 py-2 text-sm leading-relaxed",
              message.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground",
            )}
            dangerouslySetInnerHTML={{ __html: html }}
          />
        )}
      </div>
    </div>
  );
}
