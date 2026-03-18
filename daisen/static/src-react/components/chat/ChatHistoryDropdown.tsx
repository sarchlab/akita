import { useEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent } from "react";
import type { ChatConversation } from "../../types/chat";

interface ChatHistoryDropdownProps {
  chatHistory: ChatConversation[];
  currentChatId: string;
  onLoadChat: (id: string) => void;
  onDeleteChat: (id: string) => void;
}

const formatTimestamp = (timestamp: number): string => {
  const date = new Date(timestamp);
  const now = new Date();
  const isToday = date.toDateString() === now.toDateString();

  if (isToday) {
    return `Today ${date.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    })}`;
  }

  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
};

export default function ChatHistoryDropdown({
  chatHistory,
  currentChatId,
  onLoadChat,
  onDeleteChat,
}: ChatHistoryDropdownProps) {
  const [isOpen, setIsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const sortedHistory = useMemo(
    () => [...chatHistory].sort((a, b) => b.timestamp - a.timestamp),
    [chatHistory],
  );

  useEffect(() => {
    if (!isOpen) return;

    const handleDocumentClick = (event: globalThis.MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Node)) return;
      if (menuRef.current?.contains(target)) return;

      setIsOpen(false);
    };

    document.addEventListener("mousedown", handleDocumentClick);
    return () => document.removeEventListener("mousedown", handleDocumentClick);
  }, [isOpen]);

  if (sortedHistory.length === 0) {
    return <small className="text-muted">No archived conversations yet.</small>;
  }

  return (
    <div className="position-relative" ref={menuRef}>
      <button
        className="btn btn-outline-secondary btn-sm dropdown-toggle"
        onClick={() => setIsOpen((prev) => !prev)}
        type="button"
      >
        Chat History
      </button>

      {isOpen && (
        <div
          className="dropdown-menu show p-2"
          style={{ maxHeight: "260px", minWidth: "320px", overflowY: "auto" }}
        >
          {sortedHistory.map((chat) => {
            const active = chat.id === currentChatId;

            return (
              <div
                className={`d-flex align-items-start gap-2 rounded px-2 py-1 mb-1 ${
                  active ? "bg-primary text-white" : "bg-light"
                }`}
                key={chat.id}
                onClick={() => {
                  onLoadChat(chat.id);
                  setIsOpen(false);
                }}
                role="button"
              >
                <div className="flex-grow-1 overflow-hidden">
                  <div className="small fw-semibold text-truncate">{chat.title}</div>
                  <div className={`small ${active ? "text-white-50" : "text-muted"}`}>
                    {formatTimestamp(chat.timestamp)}
                  </div>
                </div>

                <button
                  aria-label={`Delete ${chat.title}`}
                  className={`btn btn-sm py-0 px-2 ${active ? "btn-light" : "btn-outline-danger"}`}
                  onClick={(event: ReactMouseEvent<HTMLButtonElement>) => {
                    event.stopPropagation();
                    onDeleteChat(chat.id);
                  }}
                  type="button"
                >
                  ×
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
