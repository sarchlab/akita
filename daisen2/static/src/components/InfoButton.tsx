import { useEffect, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Info, X } from "lucide-react";
import { cn } from "../lib/utils";

interface InfoButtonProps {
  // Short label of what is being explained — the modal title and the button's
  // accessible name.
  title: string;
  // The help content (paragraphs, lists, definition rows).
  children: ReactNode;
  // Extra classes for the icon button (e.g. sizing/color in a dense header, or a
  // translucent background + padding when overlaid in a chart corner).
  className?: string;
}

// InfoButton is a small "i" affordance for in-page help: clicking it opens a modal
// explaining a hard-to-grasp concept. The modal mirrors the chat Lightbox (portal
// to <body>, closes on Escape / backdrop / the X) so no dialog dependency is needed.
export default function InfoButton({ title, children, className }: InfoButtonProps) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  return (
    <>
      <button
        type="button"
        className={cn(
          "inline-flex shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
          className,
        )}
        title={`What is this? — ${title}`}
        aria-label={`Help: ${title}`}
        onClick={(event) => {
          // The button often sits inside a clickable card/link; keep the click local.
          event.preventDefault();
          event.stopPropagation();
          setOpen(true);
        }}
      >
        <Info className="h-4 w-4" />
      </button>

      {open
        ? createPortal(
            <div
              className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
              role="dialog"
              aria-modal="true"
              aria-label={title}
              onClick={() => setOpen(false)}
              // The portal keeps the trigger's React ancestors, so without this a wheel
              // inside the modal bubbles to a chart's onWheel (which preventDefaults to
              // zoom) — breaking modal scroll and zooming the chart behind it.
              onWheel={(event) => event.stopPropagation()}
            >
              <div
                className="flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-lg border bg-white shadow-xl"
                onClick={(event) => event.stopPropagation()}
              >
                <div className="flex shrink-0 items-start justify-between gap-3 border-b px-4 py-3">
                  <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                    <Info className="h-4 w-4 text-primary" aria-hidden="true" />
                    {title}
                  </h2>
                  <button
                    type="button"
                    className="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    onClick={() => setOpen(false)}
                    aria-label="Close help"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>
                <div className="space-y-2 overflow-auto px-4 py-3 text-sm leading-relaxed text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:text-[0.85em] [&_code]:text-foreground [&_li]:ml-4 [&_li]:list-disc [&_strong]:font-medium [&_strong]:text-foreground">
                  {children}
                </div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
