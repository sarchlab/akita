import { useEffect, useState } from "react";
import type { ChatConversation } from "../../types/chat";

interface ChatHeaderProps {
  chatHistory: ChatConversation[];
  currentChatId: string;
  onNewChat: () => void;
  onLoadChat: (id: string) => void;
  onDeleteChat: (id: string) => void;
  onClose: () => void;
}

const formatTimestamp = (timestamp: number): string =>
  new Date(timestamp).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

export default function ChatHeader({
  chatHistory,
  currentChatId,
  onNewChat,
  onLoadChat,
  onDeleteChat,
  onClose,
}: ChatHeaderProps) {
  const [selectedHistoryId, setSelectedHistoryId] = useState<string>("");

  useEffect(() => {
    if (chatHistory.length === 0) {
      setSelectedHistoryId("");
      return;
    }

    if (!chatHistory.some((chat) => chat.id === selectedHistoryId)) {
      setSelectedHistoryId(chatHistory[0].id);
    }
  }, [chatHistory, selectedHistoryId]);

  return (
    <div className="border-bottom bg-light p-2 d-flex flex-column gap-2">
      <div className="d-flex align-items-center justify-content-between gap-2">
        <div>
          <h6 className="mb-0">AI Assistant</h6>
          <small className="text-muted">Chat ID: {currentChatId.slice(-8)}</small>
        </div>

        <div className="d-flex gap-2">
          <button className="btn btn-primary btn-sm" onClick={onNewChat} type="button">
            New Chat
          </button>
          <button
            aria-label="Close chat panel"
            className="btn btn-outline-secondary btn-sm"
            onClick={onClose}
            type="button"
          >
            ×
          </button>
        </div>
      </div>

      {chatHistory.length > 0 ? (
        <div className="d-flex align-items-center gap-2">
          <select
            className="form-select form-select-sm"
            value={selectedHistoryId}
            onChange={(event) => setSelectedHistoryId(event.target.value)}
          >
            {chatHistory.map((chat) => (
              <option key={chat.id} value={chat.id}>
                {chat.title} · {formatTimestamp(chat.timestamp)}
              </option>
            ))}
          </select>

          <button
            className="btn btn-outline-primary btn-sm"
            disabled={!selectedHistoryId}
            onClick={() => selectedHistoryId && onLoadChat(selectedHistoryId)}
            type="button"
          >
            Load
          </button>
          <button
            className="btn btn-outline-danger btn-sm"
            disabled={!selectedHistoryId}
            onClick={() => selectedHistoryId && onDeleteChat(selectedHistoryId)}
            type="button"
          >
            Delete
          </button>
        </div>
      ) : (
        <small className="text-muted">No archived conversations yet.</small>
      )}
    </div>
  );
}
