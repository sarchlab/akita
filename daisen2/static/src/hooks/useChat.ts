import { useCallback, useMemo, useState } from "react";
import type { AgentStep, ChatMessage, LLMSettings, TraceInformation, UnitContent, UploadedFile } from "../types/chat";

function contentTitle(content: UnitContent[]) {
  const firstText = content.find((unit) => unit.type === "text");
  if (!firstText || firstText.type !== "text") return "New Chat";
  const words = firstText.text.trim().split(/\s+/).slice(0, 6);
  return words.join(" ") + (words.length === 6 ? "..." : "");
}

export function useChat() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatHistory, setChatHistory] = useState<
    { id: string; title: string; messages: ChatMessage[]; timestamp: number }[]
  >([{ id: "chat_1", title: "New Chat", messages: [], timestamp: Date.now() }]);
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
    const chat = { id, title: "New Chat", messages: [], timestamp: Date.now() };
    setChatHistory((history) => [...history, chat]);
    setCurrentChatId(id);
    setMessages([]);
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
          const next = remaining[0] ?? { id: "chat_1", title: "New Chat", messages: [], timestamp: Date.now() };
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
    async (content: UnitContent[], traceInfo: TraceInformation, llm: LLMSettings) => {
      const userMessage: ChatMessage = { role: "user", content };
      const nextMessages = [...messages, userMessage];
      setMessages(nextMessages);
      setLoading(true);
      setError(null);

      try {
        const headers: Record<string, string> = { "Content-Type": "application/json" };
        // The key travels in a header (not the body) so it stays out of request
        // logs. It may be omitted for keyless local servers.
        if (llm.apiKey.trim()) headers["X-Llm-Api-Key"] = llm.apiKey.trim();

        const response = await fetch("/api/gpt", {
          method: "POST",
          headers,
          body: JSON.stringify({
            messages: nextMessages,
            traceInfo,
            provider: llm.provider,
            baseURL: llm.baseURL,
            model: llm.model,
            // Agent mode: the server runs a streamed tool-calling loop and replies
            // with Server-Sent Events. It falls back to a single answer internally
            // for models without tool support.
            agent: true,
          }),
        });

        const isStream = response.ok && response.body &&
          (response.headers.get("content-type") ?? "").includes("text/event-stream");

        if (!isStream) {
          // Error or a non-streamed body — surface it as the assistant message.
          const text = await response.text();
          const finalMessages: ChatMessage[] = [
            ...nextMessages,
            { role: "assistant", content: [{ type: "text", text: text || "No response from Daisen Bot." }] },
          ];
          setMessages(finalMessages);
          saveCurrent(finalMessages);
          return;
        }

        const reader = response.body!.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        const steps: AgentStep[] = [];
        let finalText = "";

        const render = (): ChatMessage[] => {
          const assistantMessage: ChatMessage = {
            role: "assistant",
            content: [{ type: "text", text: finalText }],
            steps: steps.length ? steps.map((s) => ({ ...s })) : undefined,
          };
          const working = [...nextMessages, assistantMessage];
          setMessages(working);
          return working;
        };

        let working = render();
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          let sep: number;
          while ((sep = buffer.indexOf("\n\n")) !== -1) {
            const rawEvent = buffer.slice(0, sep);
            buffer = buffer.slice(sep + 2);
            const dataLine = rawEvent.split("\n").find((l) => l.startsWith("data:"));
            if (!dataLine) continue;
            let ev: { type: string; tool?: string; args?: string; observation?: string; text?: string; error?: string };
            try {
              ev = JSON.parse(dataLine.slice(5).trim());
            } catch {
              continue;
            }
            if (ev.type === "thinking") steps.push({ thinking: ev.text });
            else if (ev.type === "step") steps.push({ tool: ev.tool ?? "tool", args: ev.args });
            else if (ev.type === "observation") {
              const last = steps[steps.length - 1];
              if (last && last.tool) last.observation = ev.observation;
            } else if (ev.type === "message") finalText = ev.text ?? finalText;
            else if (ev.type === "error") finalText = (finalText ? finalText + "\n\n" : "") + "Error: " + (ev.error ?? "unknown");
            working = render();
          }
        }
        saveCurrent(working);
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
