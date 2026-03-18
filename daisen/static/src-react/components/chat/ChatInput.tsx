import html2canvas from "html2canvas";
import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import type { TraceInformation, UnitContent, UploadedFile } from "../../types/chat";
import FileUploadArea from "./FileUploadArea";
import TraceAttachment from "./TraceAttachment";
import GitHubAttachment from "./GitHubAttachment";

interface ChatInputProps {
  loading: boolean;
  uploadedFiles: UploadedFile[];
  onSend: (
    content: UnitContent[],
    traceInfo: TraceInformation,
    selectedGitHubRoutineKeys: string[],
  ) => Promise<void>;
  onAddFiles: (files: FileList | File[], type: UploadedFile["type"]) => Promise<void>;
  onRemoveFile: (id: number) => void;
}

const DEFAULT_TRACE_INFO: TraceInformation = {
  selected: 0,
  startTime: 0,
  endTime: 0,
  selectedComponentNameList: [],
};

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
  const [traceInfo, setTraceInfo] = useState<TraceInformation>(DEFAULT_TRACE_INFO);
  const [selectedGitHubRoutineKeys, setSelectedGitHubRoutineKeys] = useState<string[]>([]);
  const [captureLoading, setCaptureLoading] = useState(false);
  const [captureError, setCaptureError] = useState<string | null>(null);

  const textareaRef = useRef<HTMLTextAreaElement>(null);

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
    if (loading || captureLoading) return;

    const contentUnits = toContentUnits(text, uploadedFiles);
    if (contentUnits.length === 0) return;

    await onSend(contentUnits, traceInfo, selectedGitHubRoutineKeys);
    setText("");
  };

  const handleKeyDown = async (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== "Enter" || event.shiftKey) return;

    event.preventDefault();
    await send();
  };

  const captureScreenshot = async () => {
    if (loading || captureLoading) return;

    setCaptureError(null);
    setCaptureLoading(true);

    try {
      const canvas = await html2canvas(document.body);
      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));

      if (!blob) {
        throw new Error("Failed to capture screenshot.");
      }

      const screenshotFile = new File([blob], `screenshot-${Date.now()}.png`, {
        type: "image/png",
      });

      await onAddFiles([screenshotFile], "image-screenshot");
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : String(err);
      setCaptureError(message);
    } finally {
      setCaptureLoading(false);
    }
  };

  return (
    <div className="border-top p-2">
      <FileUploadArea
        loading={loading || captureLoading}
        onAddFiles={onAddFiles}
        onRemoveFile={onRemoveFile}
        uploadedFiles={uploadedFiles}
      />

      <div className="mb-2 d-flex flex-column gap-2">
        <TraceAttachment disabled={loading || captureLoading} onChange={setTraceInfo} />
        <GitHubAttachment
          disabled={loading || captureLoading}
          onChange={setSelectedGitHubRoutineKeys}
        />
      </div>

      {captureError && <div className="alert alert-warning py-1 px-2 small">{captureError}</div>}

      <textarea
        ref={textareaRef}
        className="form-control mb-2"
        disabled={loading || captureLoading}
        onChange={(event) => setText(event.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Type a message..."
        rows={1}
        value={text}
      />

      <div className="d-flex justify-content-between align-items-center gap-2">
        <button
          className="btn btn-outline-secondary btn-sm"
          disabled={loading || captureLoading}
          onClick={() => void captureScreenshot()}
          type="button"
        >
          {captureLoading ? "Capturing..." : "Screenshot"}
        </button>

        <button
          className="btn btn-primary btn-sm"
          disabled={loading || captureLoading}
          onClick={() => void send()}
          type="button"
        >
          Send
        </button>
      </div>
    </div>
  );
}
