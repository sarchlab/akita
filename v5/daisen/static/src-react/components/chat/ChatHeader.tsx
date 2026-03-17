import type { ChatConversation } from "../../types/chat";
import ChatHistoryDropdown from "./ChatHistoryDropdown";

interface ChatHeaderProps {
  chatHistory: ChatConversation[];
  currentChatId: string;
  onNewChat: () => void;
  onLoadChat: (id: string) => void;
  onDeleteChat: (id: string) => void;
  onClose: () => void;
}

export default function ChatHeader({
  chatHistory,
  currentChatId,
  onNewChat,
  onLoadChat,
  onDeleteChat,
  onClose,
}: ChatHeaderProps) {
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

      <div className="d-flex justify-content-between align-items-center">
        <ChatHistoryDropdown
          chatHistory={chatHistory}
          currentChatId={currentChatId}
          onDeleteChat={onDeleteChat}
          onLoadChat={onLoadChat}
        />
      </div>
    </div>
  );
}
