import * as React from "react";
import { cn } from "../../lib/utils";

// SidePanel is the shared docked side-panel surface used across Daisen — the
// DaisenBot chat panel, the component-view legend, and the task-view detail
// column. It docks flush against the content with a left border and the card
// surface token (white), so every side panel reads as the same element instead
// of each rolling its own gray/rounded/floating card. Callers add their own
// width, flex/scroll behavior, and padding.
const SidePanel = React.forwardRef<HTMLElement, React.HTMLAttributes<HTMLElement>>(
  ({ className, ...props }, ref) => (
    <aside ref={ref} className={cn("h-full shrink-0 border-l bg-card", className)} {...props} />
  ),
);
SidePanel.displayName = "SidePanel";

export { SidePanel };
