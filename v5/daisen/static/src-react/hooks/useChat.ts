import { useCallback, useRef, useState } from "react";
import type {
  ChatConversation,
  ChatMessage,
  GPTRequest,
  GPTResponse,
  TraceInformation,
  UnitContent,
  UploadedFile,
} from "../types/chat";

const DEFAULT_TRACE_INFO: TraceInformation = {
  selected: 0,
  startTime: 0,
  endTime: 0,
  selectedComponentNameList: [],
};

const createChatId = (): string =>
  `chat-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

const fileSizeString = (size: number): string => {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
};

const getConversationTitle = (messages: ChatMessage[]): string => {
  for (const message of messages) {
    if (message.role !== "user") continue;

    for (const content of message.content) {
      if (content.type !== "text") continue;

      const title = content.text.trim().replace(/\s+/g, " ");
      if (title.length === 0) continue;
      return title.length > 50 ? `${title.slice(0, 47)}...` : title;
    }
  }

  return "Untitled chat";
};

const readAsDataUrl = async (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error ?? new Error("Failed to read image file"));
    reader.readAsDataURL(file);
  });

const toUploadedFile = async (
  file: File,
  id: number,
  type: UploadedFile["type"],
): Promise<UploadedFile> => {
  const content = type === "file" ? await file.text() : await readAsDataUrl(file);

  return {
    id,
    name: file.name,
    content,
    type,
    size: fileSizeString(file.size),
  };
};

export interface UseChatResult {
  messages: ChatMessage[];
  chatHistory: ChatConversation[];
  currentChatId: string;
  uploadedFiles: UploadedFile[];
  loading: boolean;
  error: string | null;
  sendMessage: (
    content: UnitContent[],
    traceInfo?: TraceInformation,
    githubKeys?: string[],
  ) => Promise<void>;
  newChat: () => void;
  loadChat: (id: string) => void;
  deleteChat: (id: string) => void;
  addUploadedFiles: (files: FileList | File[], type?: UploadedFile["type"]) => Promise<void>;
  removeUploadedFile: (id: number) => void;
  clearUploadedFiles: () => void;
}

export function useChat(): UseChatResult {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatHistory, setChatHistory] = useState<ChatConversation[]>([]);
  const [currentChatId, setCurrentChatId] = useState<string>(createChatId());
  const [uploadedFiles, setUploadedFiles] = useState<UploadedFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const nextFileIdRef = useRef(1);

  const archiveCurrentConversation = useCallback(() => {
    if (messages.length === 0) return;

    const archivedConversation: ChatConversation = {
      id: currentChatId,
      title: getConversationTitle(messages),
      messages,
      timestamp: Date.now(),
    };

    setChatHistory((prev) => [archivedConversation, ...prev.filter((chat) => chat.id !== currentChatId)]);
  }, [currentChatId, messages]);

  const sendMessage = useCallback(
    async (
      content: UnitContent[],
      traceInfo: TraceInformation = DEFAULT_TRACE_INFO,
      githubKeys: string[] = [],
    ) => {
      if (loading || content.length === 0) return;

      const userMessage: ChatMessage = { role: "user", content };
      const requestMessages = [...messages, userMessage];

      setMessages(requestMessages);
      setLoading(true);
      setError(null);

      try {
        const requestPayload: GPTRequest = {
          messages: requestMessages,
          traceInfo,
          selectedGitHubRoutineKeys: githubKeys,
        };

        const response = await fetch("/api/gpt", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(requestPayload),
        });

        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }

        let assistantText = "";
        const responseType = response.headers.get("content-type") ?? "";

        if (responseType.includes("application/json")) {
          const data = (await response.json()) as GPTResponse | { content?: unknown } | string;
          if (typeof data === "string") {
            assistantText = data;
          } else if (typeof data.content === "string") {
            assistantText = data.content;
          } else {
            assistantText = JSON.stringify(data);
          }
        } else {
          assistantText = await response.text();
        }

        setMessages((prev) => [
          ...prev,
          {
            role: "assistant",
            content: [{ type: "text", text: assistantText }],
          },
        ]);
      } catch (err: unknown) {
        const message = err instanceof Error ? err.message : String(err);
        setError(message);
        setMessages((prev) => [
          ...prev,
          {
            role: "assistant",
            content: [{ type: "text", text: `Request failed: ${message}` }],
          },
        ]);

        throw err;
      } finally {
        setLoading(false);
      }
    },
    [loading, messages],
  );

  const newChat = useCallback(() => {
    archiveCurrentConversation();
    setMessages([]);
    setUploadedFiles([]);
    setCurrentChatId(createChatId());
    setError(null);
  }, [archiveCurrentConversation]);

  const loadChat = useCallback(
    (id: string) => {
      const targetConversation = chatHistory.find((chat) => chat.id === id);
      if (!targetConversation) return;

      if (messages.length > 0 && currentChatId !== id) {
        const currentConversation: ChatConversation = {
          id: currentChatId,
          title: getConversationTitle(messages),
          messages,
          timestamp: Date.now(),
        };

        setChatHistory((prev) => [
          currentConversation,
          ...prev.filter((chat) => chat.id !== currentChatId && chat.id !== id),
        ]);
      } else {
        setChatHistory((prev) => prev.filter((chat) => chat.id !== id));
      }

      setMessages(targetConversation.messages);
      setCurrentChatId(targetConversation.id);
      setUploadedFiles([]);
      setError(null);
    },
    [chatHistory, currentChatId, messages],
  );

  const deleteChat = useCallback(
    (id: string) => {
      setChatHistory((prev) => prev.filter((chat) => chat.id !== id));

      if (id === currentChatId) {
        setMessages([]);
        setCurrentChatId(createChatId());
        setUploadedFiles([]);
      }
    },
    [currentChatId],
  );

  const addUploadedFiles = useCallback(
    async (files: FileList | File[], type: UploadedFile["type"] = "file") => {
      const fileList = Array.from(files as ArrayLike<File>);
      if (fileList.length === 0) return;

      const nextFiles = await Promise.all(
        fileList.map((file) => {
          const nextId = nextFileIdRef.current;
          nextFileIdRef.current += 1;
          return toUploadedFile(file, nextId, type);
        }),
      );

      setUploadedFiles((prev) => [...prev, ...nextFiles]);
    },
    [],
  );

  const removeUploadedFile = useCallback((id: number) => {
    setUploadedFiles((prev) => prev.filter((file) => file.id !== id));
  }, []);

  const clearUploadedFiles = useCallback(() => {
    setUploadedFiles([]);
  }, []);

  return {
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
  };
}
