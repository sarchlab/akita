import { useCallback, useMemo, useState } from "react";
import type { ChatMessage, LLMSettings, TraceInformation, UnitContent, UploadedFile } from "../types/chat";

const INITIAL_MESSAGE: ChatMessage = {
  role: "assistant",
  content: [{ type: "text", text: "Hello! What can I help you with today?" }],
};

function contentTitle(content: UnitContent[]) {
  const firstText = content.find((unit) => unit.type === "text");
  if (!firstText || firstText.type !== "text") return "New Chat";
  const words = firstText.text.trim().split(/\s+/).slice(0, 6);
  return words.join(" ") + (words.length === 6 ? "..." : "");
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([INITIAL_MESSAGE]);
  const [chatHistory, setChatHistory] = useState<
    { id: string; title: string; messages: ChatMessage[]; timestamp: number }[]
  >([{ id: "chat_1", title: "New Chat", messages: [INITIAL_MESSAGE], timestamp: Date.now() }]);
  const [currentChatId, setCurrentChatId] = useState("chat_1");
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const saveCurrent = useCallback(
    (nextMessages = messages) => {
      setChatHistory((history) =>
        history.map((chat) =>
          chat.id === currentChatId
            ? {
                ...chat,
                messages: nextMessages,
                title: nextMessages.some((m) => m.role === "user")
                  ? contentTitle(nextMessages.find((m) => m.role === "user")?.content ?? [])
                  : chat.title,
                timestamp: Date.now(),
              }
            : chat,
        ),
      );
    },
    [currentChatId, messages],
  );

  const newChat = useCallback(() => {
    saveCurrent();
    const id = `chat_${Date.now()}`;
    const chat = { id, title: "New Chat", messages: [INITIAL_MESSAGE], timestamp: Date.now() };
    setChatHistory((history) => [...history, chat]);
    setCurrentChatId(id);
    setMessages([INITIAL_MESSAGE]);
  }, [saveCurrent]);

  const loadChat = useCallback(
    (id: string) => {
      saveCurrent();
      const chat = chatHistory.find((entry) => entry.id === id);
      if (!chat) return;
      setCurrentChatId(id);
      setMessages(chat.messages);
    },
    [chatHistory, saveCurrent],
  );

  const deleteChat = useCallback(
    (id: string) => {
      setChatHistory((history) => {
        const remaining = history.filter((entry) => entry.id !== id);
        if (id === currentChatId) {
          const next = remaining[0] ?? { id: "chat_1", title: "New Chat", messages: [INITIAL_MESSAGE], timestamp: Date.now() };
          setCurrentChatId(next.id);
          setMessages(next.messages);
          return remaining.length ? remaining : [next];
        }
        return remaining;
      });
    },
    [currentChatId],
  );

  const sendMessage = useCallback(
    async (
      content: UnitContent[],
      traceInfo: TraceInformation,
      selectedGitHubRoutineKeys: string[],
      llm: LLMSettings,
    ) => {
      const userMessage: ChatMessage = { role: "user", content };
      const nextMessages = [...messages, userMessage];
      setMessages(nextMessages);
      setLoading(true);
      setError(null);

      try {
        const headers: Record<string, string> = { "Content-Type": "application/json" };
        // The key travels in a header (not the body) so it stays out of request
        // logs; the server falls back to its .env when the header is absent.
        if (llm.apiKey.trim()) headers["X-LLM-Api-Key"] = llm.apiKey.trim();

        const response = await fetch("/api/gpt", {
          method: "POST",
          headers,
          body: JSON.stringify({
            messages: nextMessages,
            traceInfo,
            selectedGitHubRoutineKeys,
            provider: llm.provider,
            baseURL: llm.baseURL,
            model: llm.model,
          }),
        });
        const data = response.ok ? await response.json() : { choices: [{ message: { content: await response.text() } }] };
        const assistantText = data?.choices?.[0]?.message?.content ?? "No response from Daisen Bot.";
        const assistantMessage: ChatMessage = {
          role: "assistant",
          content: [{ type: "text", text: assistantText }],
        };
        const finalMessages = [...nextMessages, assistantMessage];
        setMessages(finalMessages);
        saveCurrent(finalMessages);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    },
    [messages, saveCurrent],
  );

  const addUploadedFiles = useCallback((files: UploadedFile[]) => {
    setUploadedFiles((current) => [...current, ...files]);
  }, []);

  const removeUploadedFile = useCallback((id: number) => {
    setUploadedFiles((current) => current.filter((file) => file.id !== id));
  }, []);

  const clearUploadedFiles = useCallback(() => setUploadedFiles([]), []);

  return useMemo(
    () => ({
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
    }),
    [
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
    ],
  );
}
