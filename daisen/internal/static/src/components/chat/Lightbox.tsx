import { useEffect } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { toSafeDaisenUrl } from "../../utils/captureView";

// One enlarged view-evidence image, plus the Daisen view URL it came from.
export interface LightboxImage {
  src: string;
  viewUrl: string;
}

// A full-window overlay for an enlarged chat-evidence thumbnail. Rendered via a
// portal to document.body so it covers the whole window rather than being clipped
// by the docked chat SidePanel. Shows the image and a link that opens the exact
// Daisen view in a new tab; closes on Escape, the close button, or backdrop click.
export default function Lightbox({
  image,
  onClose,
}: {
  image: LightboxImage | null;
  onClose: () => void;
}) {
  useEffect(() => {
    if (!image) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [image, onClose]);

  if (!image) return null;

  // The viewUrl is already a canonical Daisen path, but re-validate before exposing
  // it as a link (defense in depth — the source text is model-generated).
  let href: string | null = null;
  try {
    href = toSafeDaisenUrl(image.viewUrl);
  } catch {
    href = null;
  }

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-3 bg-black/80 p-6"
      role="dialog"
      aria-modal="true"
      onClick={onClose}
    >
      <img
        src={image.src}
        alt="Enlarged Daisen view"
        className="max-h-[80vh] max-w-[90vw] rounded-lg border border-white/20 object-contain shadow-2xl"
        onClick={(event) => event.stopPropagation()}
      />
      <div className="flex items-center gap-3" onClick={(event) => event.stopPropagation()}>
        {href ? (
          <a
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            className="rounded-md bg-white/10 px-3 py-1.5 text-sm font-medium text-white underline-offset-2 hover:bg-white/20"
          >
            Open this view in a new tab ↗
          </a>
        ) : null}
        <button
          type="button"
          onClick={onClose}
          className="inline-flex items-center gap-1 rounded-md bg-white/10 px-3 py-1.5 text-sm text-white hover:bg-white/20"
        >
          <X className="h-4 w-4" /> Close
        </button>
      </div>
    </div>,
    document.body,
  );
}
