import { useState } from "react";
import type { UnitContent } from "../../types/chat";
import { useChat } from "../../hooks/useChat";
import ChatHeader from "./ChatHeader";
import MessageList from "./MessageList";
import ChatInput from "./ChatInput";

export default function ChatPanel() {
  const [isOpen, setIsOpen] = useState(false);
  const {
    messages,
    chatHistory,
    currentChatId,
    uploadedFiles,
    loading,
    error,
    sendMessage,
    newChat,
    loadChat,
    deleteChat,
    addUploadedFiles,
    removeUploadedFile,
    clearUploadedFiles,
  } = useChat();

  const handleSend = async (content: UnitContent[]) => {
    try {
      await sendMessage(content);
      clearUploadedFiles();
    } catch {
      // Error state is managed in useChat.
    }
  };

  return (
    <>
      {!isOpen && (
        <button
          className="btn btn-primary rounded-pill shadow"
          onClick={() => setIsOpen(true)}
          style={{ bottom: "1rem", position: "fixed", right: "1rem", zIndex: 1035 }}
          type="button"
        >
          AI Chat
        </button>
      )}

      <aside
        className="bg-white border-start shadow d-flex flex-column"
        style={{
          bottom: 0,
          height: "calc(100vh - 56px)",
          maxWidth: "100vw",
          position: "fixed",
          right: 0,
          top: "56px",
          transform: isOpen ? "translateX(0)" : "translateX(100%)",
          transition: "transform 0.2s ease-in-out",
          width: "400px",
          zIndex: 1040,
        }}
      >
        <ChatHeader
          chatHistory={chatHistory}
          currentChatId={currentChatId}
          onClose={() => setIsOpen(false)}
          onDeleteChat={deleteChat}
          onLoadChat={loadChat}
          onNewChat={newChat}
        />

        <MessageList loading={loading} messages={messages} />

        {error && <div className="alert alert-danger rounded-0 mb-0 py-1 px-2 small">{error}</div>}

        <ChatInput
          loading={loading}
          onAddFiles={addUploadedFiles}
          onRemoveFile={removeUploadedFile}
          onSend={handleSend}
          uploadedFiles={uploadedFiles}
        />
      </aside>
    </>
  );
}
