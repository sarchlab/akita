import { useCallback, useRef } from "react";

export type DividerOrientation = "vertical" | "horizontal";

interface ResizableDividerProps {
  /** Divider orientation — vertical separates left/right, horizontal separates top/bottom. */
  orientation: DividerOrientation;
  /** Thickness of the draggable divider in pixels. */
  size?: number;
  /** Called continuously while dragging, with the mouse clientX or clientY. */
  onDrag: (clientPos: number) => void;
  /** Called when drag ends. */
  onDragEnd?: () => void;
}

/**
 * ResizableDivider — a draggable bar that separates two resizable panels.
 *
 * Handles mousedown → document mousemove/mouseup lifecycle to allow smooth
 * resizing even when the cursor drifts outside the divider element.
 */
export default function ResizableDivider({
  orientation,
  size = 8,
  onDrag,
  onDragEnd,
}: ResizableDividerProps) {
  const draggingRef = useRef(false);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      draggingRef.current = true;

      const handleMouseMove = (ev: MouseEvent) => {
        if (!draggingRef.current) return;
        const pos =
          orientation === "vertical" ? ev.clientX : ev.clientY;
        onDrag(pos);
      };

      const handleMouseUp = () => {
        draggingRef.current = false;
        document.removeEventListener("mousemove", handleMouseMove);
        document.removeEventListener("mouseup", handleMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
        onDragEnd?.();
      };

      document.addEventListener("mousemove", handleMouseMove);
      document.addEventListener("mouseup", handleMouseUp);

      // Prevent text selection and set resize cursor on body during drag
      document.body.style.cursor =
        orientation === "vertical" ? "col-resize" : "row-resize";
      document.body.style.userSelect = "none";
    },
    [orientation, onDrag, onDragEnd],
  );

  const isVertical = orientation === "vertical";

  return (
    <div
      onMouseDown={handleMouseDown}
      style={{
        width: isVertical ? size : "100%",
        height: isVertical ? "100%" : size,
        cursor: isVertical ? "col-resize" : "row-resize",
        backgroundColor: "#dee2e6",
        flexShrink: 0,
        zIndex: 10,
      }}
      role="separator"
      aria-orientation={orientation}
    />
  );
}
