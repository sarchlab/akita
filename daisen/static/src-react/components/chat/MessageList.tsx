import { useEffect, useRef } from "react";
import type { ChatMessage } from "../../types/chat";
import MessageBubble from "./MessageBubble";

interface MessageListProps {
  messages: ChatMessage[];
  loading: boolean;
}

export default function MessageList({ messages, loading }: MessageListProps) {
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const list = listRef.current;
    if (!list) return;

    list.scrollTop = list.scrollHeight;
  }, [messages, loading]);

  return (
    <div ref={listRef} className="flex-grow-1 overflow-auto p-3 bg-body-tertiary">
      {messages.length === 0 ? (
        <p className="text-muted small mb-0">Ask a question to start chatting.</p>
      ) : (
        messages.map((message, index) => (
          <MessageBubble key={`msg-${index}-${message.role}`} message={message} />
        ))
      )}

      {loading && (
        <div className="d-flex justify-content-start mt-2">
          <div className="spinner-border spinner-border-sm text-secondary" role="status">
            <span className="visually-hidden">Loading...</span>
          </div>
        </div>
      )}
    </div>
  );
}
