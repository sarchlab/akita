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

  return (
    <div className={cn("flex", message.role === "user" ? "justify-end" : "justify-start")}>
      <div
        ref={ref}
        className={cn(
          "chat-markdown max-w-[92%] rounded-2xl px-3 py-2 text-sm leading-relaxed",
          message.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground",
        )}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
