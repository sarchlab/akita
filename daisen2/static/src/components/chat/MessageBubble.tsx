import { useEffect, useRef } from "react";
import type { ChatMessage } from "../../types/chat";
import { renderChatMarkdown, renderMathInElement } from "../../utils/chatMarkdown";
import { cn } from "../../lib/utils";

export default function MessageBubble({ message }: { message: ChatMessage }) {
  const ref = useRef<HTMLDivElement | null>(null);
  const text = message.content
    .map((unit) => (unit.type === "text" ? unit.text : "[image]"))
    .join("\n");

  useEffect(() => {
    if (ref.current) renderMathInElement(ref.current);
  }, [text]);

  return (
    <div className={cn("flex", message.role === "user" ? "justify-end" : "justify-start")}>
      <div
        ref={ref}
        className={cn(
          "chat-markdown max-w-[92%] rounded-2xl px-3 py-2 text-sm leading-relaxed",
          message.role === "user" ? "bg-primary text-primary-foreground" : "bg-muted text-foreground",
        )}
        dangerouslySetInnerHTML={{ __html: renderChatMarkdown(text) }}
      />
    </div>
  );
}
