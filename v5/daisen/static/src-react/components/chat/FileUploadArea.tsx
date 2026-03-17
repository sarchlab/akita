import { useRef, useState } from "react";
import type { ChangeEvent, DragEvent } from "react";
import type { UploadedFile } from "../../types/chat";

interface FileUploadAreaProps {
  loading: boolean;
  uploadedFiles: UploadedFile[];
  onAddFiles: (files: FileList | File[], type: UploadedFile["type"]) => Promise<void>;
  onRemoveFile: (id: number) => void;
}

const isImageFile = (file: UploadedFile): boolean =>
  file.type === "image" || file.type === "image-screenshot";

const displayFileSize = (file: UploadedFile): string => (file.size.trim().length > 0 ? file.size : "0 B");

export default function FileUploadArea({
  loading,
  uploadedFiles,
  onAddFiles,
  onRemoveFile,
}: FileUploadAreaProps) {
  const [dragOver, setDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);

  const addDroppedFiles = async (files: File[]) => {
    const imageFiles = files.filter((file) => file.type.startsWith("image/"));
    const otherFiles = files.filter((file) => !file.type.startsWith("image/"));

    if (otherFiles.length > 0) {
      await onAddFiles(otherFiles, "file");
    }

    if (imageFiles.length > 0) {
      await onAddFiles(imageFiles, "image");
    }
  };

  const handleDrop = async (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragOver(false);

    if (loading) return;

    const droppedFiles = Array.from(event.dataTransfer.files ?? []);
    if (droppedFiles.length === 0) return;

    await addDroppedFiles(droppedFiles);
  };

  const handleSelectFiles = async (
    event: ChangeEvent<HTMLInputElement>,
    type: UploadedFile["type"],
  ) => {
    if (!event.target.files || event.target.files.length === 0) return;

    await onAddFiles(event.target.files, type);
    event.target.value = "";
  };

  return (
    <div className="mb-2">
      <div className="d-flex gap-2 mb-2">
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

      <div
        className={`border rounded p-2 small ${dragOver ? "border-primary bg-primary-subtle" : "bg-light"}`}
        onDragEnter={(event) => {
          event.preventDefault();
          if (!loading) setDragOver(true);
        }}
        onDragLeave={(event) => {
          event.preventDefault();
          if (event.currentTarget.contains(event.relatedTarget as Node | null)) return;
          setDragOver(false);
        }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => void handleDrop(event)}
      >
        Drag and drop files or images here.
      </div>

      {uploadedFiles.length > 0 && (
        <div className="mt-2 d-flex flex-column gap-2">
          {uploadedFiles.map((file) => (
            <div key={file.id} className="border rounded p-2 d-flex gap-2 align-items-start bg-white">
              {isImageFile(file) ? (
                <img
                  alt={file.name}
                  className="rounded border"
                  src={file.content}
                  style={{ height: "42px", objectFit: "cover", width: "56px" }}
                />
              ) : (
                <span className="badge text-bg-secondary align-self-center">FILE</span>
              )}

              <div className="flex-grow-1 overflow-hidden">
                <div className="small fw-semibold text-truncate">{file.name}</div>
                <div className="small text-muted">{displayFileSize(file)}</div>
              </div>

              <button
                aria-label={`Remove ${file.name}`}
                className="btn btn-sm btn-outline-danger py-0 px-2"
                disabled={loading}
                onClick={() => onRemoveFile(file.id)}
                type="button"
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}

      <input
        ref={fileInputRef}
        className="d-none"
        onChange={(event) => void handleSelectFiles(event, "file")}
        type="file"
      />
      <input
        ref={imageInputRef}
        accept="image/*"
        className="d-none"
        onChange={(event) => void handleSelectFiles(event, "image")}
        type="file"
      />
    </div>
  );
}
