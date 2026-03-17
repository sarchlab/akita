import type { UploadedFile } from "../types/chat";

export const FILE_UPLOAD_EXTENSIONS = [
  ".sqlite",
  ".sqlite3",
  ".csv",
  ".txt",
  ".json",
  ".py",
  ".js",
  ".c",
  ".cpp",
  ".java",
] as const;

export const IMAGE_UPLOAD_EXTENSIONS = [".png", ".jpg", ".jpeg"] as const;

export const FILE_UPLOAD_MAX_SIZE_BYTES = 32 * 1024;
export const IMAGE_UPLOAD_MAX_SIZE_BYTES = 256 * 1024;

export const FILE_UPLOAD_ACCEPT = FILE_UPLOAD_EXTENSIONS.join(",");
export const IMAGE_UPLOAD_ACCEPT = IMAGE_UPLOAD_EXTENSIONS.join(",");

export interface UploadValidationResult {
  valid: boolean;
  error?: string;
}

export interface UploadFileLike {
  name: string;
  size: number;
  type?: string;
}

const hasAllowedExtension = (fileName: string, allowed: readonly string[]): boolean => {
  const lowerName = fileName.toLowerCase();
  return allowed.some((suffix) => lowerName.endsWith(suffix));
};

export const isImageUploadCandidate = (file: Pick<UploadFileLike, "name" | "type">): boolean => {
  const mimeType = file.type?.toLowerCase() ?? "";
  if (mimeType.startsWith("image/")) return true;
  return hasAllowedExtension(file.name, IMAGE_UPLOAD_EXTENSIONS);
};

export const validateUploadedFile = (
  file: Pick<UploadFileLike, "name" | "size">,
  type: UploadedFile["type"],
): UploadValidationResult => {
  if (type === "image-screenshot") {
    return { valid: true };
  }

  if (type === "file") {
    if (!hasAllowedExtension(file.name, FILE_UPLOAD_EXTENSIONS)) {
      return {
        valid: false,
        error:
          "Invalid file type. Allowed: .sqlite, .sqlite3, .csv, .txt, .json, .py, .js, .c, .cpp, .java",
      };
    }

    if (file.size > FILE_UPLOAD_MAX_SIZE_BYTES) {
      return {
        valid: false,
        error: "File too large. Max size is 32 KB.",
      };
    }

    return { valid: true };
  }

  if (!hasAllowedExtension(file.name, IMAGE_UPLOAD_EXTENSIONS)) {
    return {
      valid: false,
      error: "Invalid file type. Allowed: .png, .jpg, .jpeg",
    };
  }

  if (file.size > IMAGE_UPLOAD_MAX_SIZE_BYTES) {
    return {
      valid: false,
      error: "File too large. Max size is 256 KB.",
    };
  }

  return { valid: true };
};
