import { useEffect, useMemo, useRef } from "react";
import DOMPurify from "dompurify";
import type { ChatMessage } from "../../types/chat";
import { renderChatMarkdown, renderMathInElement } from "../../utils/chatMarkdown";
import { cn } from "../../lib/utils";

export default function MessageBubble({ message }: { message: ChatMessage }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const text = message.content
    .map((unit) => (unit.type === "text" ? unit.text : "[image]"))
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

  return (
    <div className={cn("flex", message.role === "user" ? "justify-end" : "justify-start")}>
      <div className="flex max-w-[92%] flex-col gap-1.5">
        {steps && steps.length > 0 && (
          <details className="rounded-xl border bg-background/60 px-2.5 py-1.5 text-xs">
            <summary className="cursor-pointer select-none text-muted-foreground">
              {steps.length} tool step{steps.length > 1 ? "s" : ""}
            </summary>
            <div className="mt-1.5 space-y-2">
              {steps.map((step, i) => (
                <div key={i} className="space-y-0.5">
                  <div className="font-medium text-foreground">{step.tool}</div>
                  {step.args && (
                    <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded bg-muted px-1.5 py-1 font-mono text-[11px]">
                      {step.args}
                    </pre>
                  )}
                  {step.observation && (
                    <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-[11px] text-muted-foreground">
                      {step.observation}
                    </pre>
                  )}
                </div>
              ))}
            </div>
          </details>
        )}
        <div
          ref={ref}
          className={cn(
            "chat-markdown rounded-2xl px-3 py-2 text-sm leading-relaxed",
            message.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground",
          )}
          dangerouslySetInnerHTML={{ __html: html }}
        />
      </div>
    </div>
  );
}
