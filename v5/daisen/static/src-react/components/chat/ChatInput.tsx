import { useEffect, useRef, useState } from "react";
import type { ChangeEvent, KeyboardEvent } from "react";
import type { UnitContent, UploadedFile } from "../../types/chat";

interface ChatInputProps {
  loading: boolean;
  uploadedFiles: UploadedFile[];
  onSend: (content: UnitContent[]) => Promise<void>;
  onAddFiles: (files: FileList, type: "file" | "image") => Promise<void>;
  onRemoveFile: (id: number) => void;
}

const toContentUnits = (text: string, files: UploadedFile[]): UnitContent[] => {
  const units: UnitContent[] = [];

  if (text.trim().length > 0) {
    units.push({ type: "text", text: text.trim() });
  }

  for (const file of files) {
    if (file.type === "image" || file.type === "image-screenshot") {
      units.push({ type: "image_url", image_url: { url: file.content } });
      continue;
    }

    units.push({
      type: "text",
      text: `Attached file: ${file.name}\n\n${file.content}`,
    });
  }

  return units;
};

export default function ChatInput({
  loading,
  uploadedFiles,
  onSend,
  onAddFiles,
  onRemoveFile,
}: ChatInputProps) {
  const [text, setText] = useState("");

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    textarea.style.height = "auto";

    const lineHeight = 24;
    const maxHeight = lineHeight * 4;
    const nextHeight = Math.min(textarea.scrollHeight, maxHeight);

    textarea.style.height = `${nextHeight}px`;
    textarea.style.overflowY = textarea.scrollHeight > maxHeight ? "auto" : "hidden";
  }, [text]);

  const send = async () => {
    if (loading) return;

    const contentUnits = toContentUnits(text, uploadedFiles);
    if (contentUnits.length === 0) return;

    await onSend(contentUnits);
    setText("");
  };

  const handleKeyDown = async (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || event.shiftKey) return;

    event.preventDefault();
    await send();
  };

  const handleFileSelection = async (
    event: ChangeEvent<HTMLInputElement>,
    type: "file" | "image",
  ) => {
    if (!event.target.files || event.target.files.length === 0) return;

    await onAddFiles(event.target.files, type);
    event.target.value = "";
  };

  return (
    <div className="border-top p-2">
      {uploadedFiles.length > 0 && (
        <div className="mb-2 d-flex flex-wrap gap-1">
          {uploadedFiles.map((file) => (
            <span key={file.id} className="badge text-bg-secondary d-flex align-items-center gap-1">
              {file.name} ({file.size})
              <button
                aria-label={`Remove ${file.name}`}
                className="btn btn-sm btn-link text-white p-0 border-0"
                disabled={loading}
                onClick={() => onRemoveFile(file.id)}
                type="button"
              >
                ×
              </button>
            </span>
          ))}
        </div>
      )}

      <textarea
        ref={textareaRef}
        className="form-control mb-2"
        disabled={loading}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Type a message..."
        rows={1}
        value={text}
      />

      <div className="d-flex justify-content-between align-items-center gap-2">
        <div className="d-flex gap-2">
          <button
            className="btn btn-outline-secondary btn-sm"
            disabled={loading}
            onClick={() => fileInputRef.current?.click()}
            type="button"
          >
            File
          </button>
          <button
            className="btn btn-outline-secondary btn-sm"
            disabled={loading}
            onClick={() => imageInputRef.current?.click()}
            type="button"
          >
            Image
          </button>
        </div>

        <button className="btn btn-primary btn-sm" disabled={loading} onClick={send} type="button">
          Send
        </button>
      </div>

      <input
        ref={fileInputRef}
        className="d-none"
        onChange={(event) => void handleFileSelection(event, "file")}
        type="file"
      />
      <input
        ref={imageInputRef}
        accept="image/*"
        className="d-none"
        onChange={(event) => void handleFileSelection(event, "image")}
        type="file"
      />
    </div>
  );
}
