import type { ReactNode, Ref } from "react";
import { SidePanel } from "./ui/side-panel";

// Single source of truth for the chart pages' docked side-panel width. Both the
// component view and the task view render their chart area + side panel through
// TraceChartLayout, so the panel width and the overall shell can't drift apart
// (which is exactly how they diverged before — one used w-96, the other 350px).
export const SIDE_PANEL_WIDTH = 360;

interface TraceChartLayoutProps {
  // The chart area (left). It sizes itself to the remaining width.
  children: ReactNode;
  // The side-panel content (right) — detail + legend, plus any page-specific
  // header (e.g. the component view's breadcrumb/location tree).
  panel: ReactNode;
  // Optional ref on the layout root, for pages that measure the full width to
  // derive their chart-area width.
  rootRef?: Ref<HTMLDivElement>;
}

export default function TraceChartLayout({ children, panel, rootRef }: TraceChartLayoutProps) {
  return (
    <div ref={rootRef} className="flex h-full overflow-hidden bg-white">
      {children}
      <SidePanel className="flex flex-col" style={{ width: SIDE_PANEL_WIDTH }}>
        {panel}
      </SidePanel>
    </div>
  );
}
