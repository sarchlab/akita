import type { TraceInformation, UnitContent } from "../../types/chat";
import { useChat } from "../../hooks/useChat";
import ChatHeader from "./ChatHeader";
import MessageList from "./MessageList";
import ChatInput from "./ChatInput";

export default function ChatPanel({
  isOpen,
  onClose,
}: {
  isOpen: boolean;
  onClose: () => void;
}) {
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

  const handleSend = async (
    content: UnitContent[],
    traceInfo: TraceInformation,
    selectedGitHubRoutineKeys: string[],
  ) => {
    try {
      await sendMessage(content, traceInfo, selectedGitHubRoutineKeys);
      clearUploadedFiles();
    } catch {
      // Error state is managed in useChat.
    }
  };

  return (
    <>
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
          onClose={onClose}
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
