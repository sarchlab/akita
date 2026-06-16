import { useEffect, useMemo, useRef } from "react";
import DOMPurify from "dompurify";
import type { ChatMessage, UnitContent } from "../../types/chat";
import { renderChatMarkdown, renderMathInElement } from "../../utils/chatMarkdown";
import { cn } from "../../lib/utils";

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

export default function MessageBubble({ message }: { message: ChatMessage }) {
  const ref = useRef<HTMLDivElement | null>(null);

  const images = message.content.filter(
    (u): u is Extract<UnitContent, { type: "image_url" }> => u.type === "image_url",
  );
  const text = message.content
    .filter((u): u is Extract<UnitContent, { type: "text" }> => u.type === "text")
    .map((u) => u.text)
    .join("\n");

  // Model output is untrusted (any configured provider, including local/unknown
  // models), so sanitize the generated HTML before injecting it. DOMPurify keeps
  // the safe markup we emit — including the `.math` spans KaTeX fills in below —
  // while stripping scripts, event handlers, and other injection vectors.
  const html = useMemo(() => DOMPurify.sanitize(renderChatMarkdown(text)), [text]);

  useEffect(() => {
    if (ref.current) renderMathInElement(ref.current);
  }, [html]);

  const steps = message.role === "assistant" ? message.steps : undefined;
  const toolCount = steps?.filter((s) => s.tool).length ?? 0;

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
            ref={ref}
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
