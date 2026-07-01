import { Minus, Plus } from "lucide-react";
import { cn } from "../../lib/utils";

// Zoom toolbar button styling, shared so every zoom toolbar reads identically
// (this control plus e.g. the gantt's row-zoom control).
export const ZOOM_BTN_CLASS = "rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-primary";

// TimeZoomControls is the horizontal (time-axis) zoom widget, rendered at the page
// level so time zoom is always available — independent of what the chart below
// shows. onZoom(dir) zooms out for dir > 0 and in for dir < 0.
export default function TimeZoomControls({
  onZoom,
  className,
}: {
  onZoom: (dir: number) => void;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "z-10 flex items-center gap-0.5 rounded border bg-white/90 px-1 py-0.5 shadow-sm",
        className,
      )}
      // stopPropagation so a click on the toolbar doesn't reach the chart's
      // pan/drag handlers (which capture the pointer and would swallow the click).
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className="select-none px-0.5 text-[10px] font-medium text-muted-foreground">time</span>
      <button type="button" className={ZOOM_BTN_CLASS} title="Zoom time out" onClick={() => onZoom(1)}>
        <Minus className="h-4 w-4" />
      </button>
      <button type="button" className={ZOOM_BTN_CLASS} title="Zoom time in" onClick={() => onZoom(-1)}>
        <Plus className="h-4 w-4" />
      </button>
    </div>
  );
}
