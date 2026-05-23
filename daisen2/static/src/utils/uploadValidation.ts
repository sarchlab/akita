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
];
export const IMAGE_UPLOAD_EXTENSIONS = [".png", ".jpg", ".jpeg"];
export const FILE_UPLOAD_ACCEPT = FILE_UPLOAD_EXTENSIONS.join(",");
export const IMAGE_UPLOAD_ACCEPT = IMAGE_UPLOAD_EXTENSIONS.join(",");

const MAX_FILE_BYTES = 32 * 1024;
const MAX_IMAGE_BYTES = 256 * 1024;

function hasAllowedExtension(name: string, extensions: string[]): boolean {
  const lowerName = name.toLowerCase();
  return extensions.some((extension) => lowerName.endsWith(extension));
}

export function isImageUploadCandidate(file: Pick<File, "name" | "type">): boolean {
  return file.type?.toLowerCase().startsWith("image/") || hasAllowedExtension(file.name, IMAGE_UPLOAD_EXTENSIONS);
}

export function validateUploadedFile(
  file: Pick<File, "name" | "size">,
  type: "file" | "image" | "image-screenshot",
): { valid: true } | { valid: false; error: string } {
  if (type === "image-screenshot") {
    return { valid: true };
  }

  if (type === "file") {
    if (!hasAllowedExtension(file.name, FILE_UPLOAD_EXTENSIONS)) {
      return { valid: false, error: `Invalid file type. Allowed: ${FILE_UPLOAD_EXTENSIONS.join(", ")}` };
    }
    if (file.size > MAX_FILE_BYTES) {
      return { valid: false, error: "File too large. Max size is 32 KB." };
    }
    return { valid: true };
  }

  if (!hasAllowedExtension(file.name, IMAGE_UPLOAD_EXTENSIONS)) {
    return { valid: false, error: `Invalid file type. Allowed: ${IMAGE_UPLOAD_EXTENSIONS.join(", ")}` };
  }
  if (file.size > MAX_IMAGE_BYTES) {
    return { valid: false, error: "File too large. Max size is 256 KB." };
  }
  return { valid: true };
}
