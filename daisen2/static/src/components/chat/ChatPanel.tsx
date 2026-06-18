import { useMemo, useRef, useState } from "react";
import { ArrowLeft, Bot, ImagePlus, Paperclip, Send, Settings, Trash2, X } from "lucide-react";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Textarea } from "../ui/textarea";
import { Alert, AlertDescription } from "../ui/alert";
import { useChat } from "../../hooks/useChat";
import { useComponentNames } from "../../hooks/useComponentNames";
import { useSimulationRange } from "../../hooks/useSimulationRange";
import { useLLMSettings } from "../../hooks/useLLMSettings";
import { useTraceId } from "../../hooks/useTraceId";
import type { TraceInformation, UploadedFile, UnitContent } from "../../types/chat";
import {
  FILE_UPLOAD_ACCEPT,
  IMAGE_UPLOAD_ACCEPT,
  isImageUploadCandidate,
  validateUploadedFile,
} from "../../utils/uploadValidation";
import MessageBubble from "./MessageBubble";
import ChatSettings from "./ChatSettings";
import { SidePanel } from "../ui/side-panel";
import Lightbox, { type LightboxImage } from "./Lightbox";
import { cn } from "../../lib/utils";

function humanSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatWhen(timestamp: number) {
  return new Date(timestamp).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function readFileAsDataUrl(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

function readFileAsText(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file);
  });
}

export default function ChatPanel({ open }: { open: boolean }) {
  const [showSettings, setShowSettings] = useState(false);
  const [lightbox, setLightbox] = useState<LightboxImage | null>(null);
  const [view, setView] = useState<"list" | "chat">("list");
  const [input, setInput] = useState("");
  const [uploadError, setUploadError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const imageInputRef = useRef<HTMLInputElement | null>(null);
  const traceId = useTraceId();
  const { names } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { settings, update, applyPreset, clearKey } = useLLMSettings();
  const {
    messages,
    chatHistory,
    loading,
    error,
    uploadedFiles,
    addUploadedFiles,
    removeUploadedFile,
    clearUploadedFiles,
    sendMessage,
    loadChat,
    deleteChat,
  } = useChat(traceId);

  // Past conversations for the selector: those with a message, newest first.
  const conversations = useMemo(
    () =>
      chatHistory
        .filter((chat) => chat.messages.some((message) => message.role === "user"))
        .sort((a, b) => b.timestamp - a.timestamp),
    [chatHistory],
  );

  const traceInfo: TraceInformation = useMemo(
    () => ({
      selected: names.length,
      startTime,
      endTime,
      selectedComponentNameList: names,
    }),
    [endTime, names, startTime],
  );

  async function handleFiles(files: FileList | null, forcedType?: "file" | "image") {
    if (!files) return;
    setUploadError(null);
    const nextFiles: UploadedFile[] = [];
    for (const file of Array.from(files)) {
      const type = forcedType ?? (isImageUploadCandidate(file) ? "image" : "file");
      const validation = validateUploadedFile(file, type);
      if (validation.valid === false) {
        setUploadError(validation.error);
        continue;
      }
      const content = type === "image" ? await readFileAsDataUrl(file) : await readFileAsText(file);
      nextFiles.push({
        id: Date.now() + nextFiles.length,
        name: file.name,
        content,
        type,
        size: humanSize(file.size),
      });
    }
    addUploadedFiles(nextFiles);
  }

  async function submit() {
    if (!input.trim() && !uploadedFiles.length) return;
    const content: UnitContent[] = [];
    const attachedText = uploadedFiles
      .filter((file) => file.type === "file")
      .map((file) => `\n\n[Attached file: ${file.name}]\n${file.content}`)
      .join("");
    content.push({ type: "text", text: `${input}${attachedText}` });
    uploadedFiles
      .filter((file) => file.type === "image" || file.type === "image-screenshot")
      .forEach((file) => content.push({ type: "image_url", image_url: { url: file.content } }));

    const startNew = view === "list";
    setInput("");
    if (startNew) setView("chat");
    await sendMessage(content, traceInfo, settings, { newConversation: startNew });
    clearUploadedFiles();
  }

  return (
    <>
      <SidePanel className={cn("flex w-[min(560px,42vw)] flex-col", !open && "hidden")}>
        <header className="flex h-14 items-center justify-between border-b px-3">
          <div className="flex items-center gap-2 font-semibold">
            {view === "chat" && !showSettings ? (
              <Button
                type="button"
                size="icon"
                variant="ghost"
                onClick={() => setView("list")}
                aria-label="Back to conversations"
              >
                <ArrowLeft />
              </Button>
            ) : null}
            <Bot className="h-5 w-5 text-primary" />
            Daisen Bot
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              size="icon"
              variant={showSettings ? "secondary" : "ghost"}
              onClick={() => setShowSettings((value) => !value)}
              aria-label="Model and provider settings"
            >
              <Settings />
            </Button>
          </div>
        </header>

        {showSettings ? (
          <ChatSettings
            settings={settings}
            update={update}
            applyPreset={applyPreset}
            clearKey={clearKey}
            onClose={() => setShowSettings(false)}
          />
        ) : (
          <>
            <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4">
              {view === "list" ? (
                <div className="space-y-1.5">
                  <div className="px-1 pb-1 text-xs font-medium text-muted-foreground">Conversations</div>
                  {conversations.length === 0 ? (
                    <div className="px-1 py-8 text-center text-sm text-muted-foreground">
                      No conversations yet. Type below to start one.
                    </div>
                  ) : (
                    conversations.map((chat) => (
                      <div
                        key={chat.id}
                        className="group flex items-center gap-2 rounded-lg border bg-background/60 px-3 py-2 hover:bg-accent"
                      >
                        <button
                          type="button"
                          className="flex min-w-0 flex-1 flex-col items-start text-left"
                          onClick={() => {
                            loadChat(chat.id);
                            setView("chat");
                          }}
                        >
                          <span className="w-full truncate text-sm font-medium">{chat.title}</span>
                          <span className="text-xs text-muted-foreground">{formatWhen(chat.timestamp)}</span>
                        </button>
                        <button
                          type="button"
                          aria-label="Delete conversation"
                          className="text-muted-foreground opacity-0 transition group-hover:opacity-100 hover:text-destructive"
                          onClick={() => deleteChat(chat.id)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      </div>
                    ))
                  )}
                </div>
              ) : (
                <>
                  <div className="text-center text-xs text-muted-foreground/70">
                    {settings.model.trim() ? `Using ${settings.model}` : "No model selected — open settings (gear icon)"}
                  </div>
                  {messages.map((message, index) => (
                    <MessageBubble key={index} message={message} onEnlarge={setLightbox} />
                  ))}
                  {loading ? <div className="text-sm text-muted-foreground">Daisen Bot is thinking...</div> : null}
                </>
              )}
            </div>

            {error ? <div className="border-t bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div> : null}

            {uploadError ? (
              <div className="border-t px-3 py-2">
                <Alert variant="destructive" className="pr-9">
                  <AlertDescription>{uploadError}</AlertDescription>
                  <button
                    type="button"
                    className="absolute right-3 top-3 text-destructive/70 hover:text-destructive"
                    onClick={() => setUploadError(null)}
                    aria-label="Dismiss upload error"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </Alert>
              </div>
            ) : null}

            {uploadedFiles.length ? (
              <div className="flex flex-wrap gap-2 border-t px-3 py-2">
                {uploadedFiles.map((file) => (
                  <span key={file.id} className="inline-flex items-center gap-2 rounded-md border bg-muted px-2 py-1 text-xs">
                    {file.name} ({file.size})
                    <button type="button" onClick={() => removeUploadedFile(file.id)} className="text-muted-foreground hover:text-foreground">
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
              </div>
            ) : null}

            <footer className="space-y-2 border-t p-3">
              <Textarea
                value={input}
                disabled={loading}
                placeholder="Ask about this trace..."
                onChange={(event) => setInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                    void submit();
                  }
                }}
              />
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Input ref={fileInputRef} className="hidden" type="file" multiple accept={FILE_UPLOAD_ACCEPT} onChange={(event) => void handleFiles(event.target.files, "file")} />
                  <Input ref={imageInputRef} className="hidden" type="file" multiple accept={IMAGE_UPLOAD_ACCEPT} onChange={(event) => void handleFiles(event.target.files, "image")} />
                  <Button type="button" variant="outline" size="sm" onClick={() => fileInputRef.current?.click()}>
                    <Paperclip />
                    File
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => imageInputRef.current?.click()}>
                    <ImagePlus />
                    Image
                  </Button>
                </div>
                <Button type="button" disabled={loading || (!input.trim() && !uploadedFiles.length)} onClick={() => void submit()}>
                  <Send />
                  Send
                </Button>
              </div>
            </footer>
          </>
        )}
      </SidePanel>
      <Lightbox image={lightbox} onClose={() => setLightbox(null)} />
    </>
  );
}
