import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Maximize2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { cn } from "../lib/utils";

interface WidgetCardProps {
  title: string;
  /** When set, an expand button links to this widget's full page. */
  expandHref?: string;
  /** Optional extra content on the right of the header (e.g. a count). */
  headerRight?: ReactNode;
  /** Extra classes for the content area (e.g. "p-0" for edge-to-edge graphs). */
  contentClassName?: string;
  /** Render the content full-bleed, without the card frame or header. Used for
   *  the enlarged single-widget pages. */
  bare?: boolean;
  children: ReactNode;
}

// WidgetCard is the shared frame for every overview widget: a titled card whose
// header carries an optional "open in full page" button. In `bare` mode it
// drops the frame entirely and just fills its container, so the enlarged page
// is the widget itself with no surrounding chrome.
export default function WidgetCard({
  title,
  expandHref,
  headerRight,
  contentClassName,
  bare,
  children,
}: WidgetCardProps) {
  if (bare) {
    return (
      <div
        className={cn(
          "flex h-full min-h-0 min-w-0 flex-col overflow-auto p-4",
          contentClassName,
        )}
      >
        {children}
      </div>
    );
  }

  return (
    <Card className="flex min-h-0 min-w-0 flex-1 flex-col">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle>{title}</CardTitle>
        <div className="flex items-center gap-3">
          {headerRight}
          {expandHref ? (
            <Link
              to={expandHref}
              title="Open in full page"
              aria-label="Open in full page"
              className="text-muted-foreground transition-colors hover:text-foreground"
            >
              <Maximize2 className="h-4 w-4" />
            </Link>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className={cn("min-h-0 flex-1 overflow-auto", contentClassName)}>
        {children}
      </CardContent>
    </Card>
  );
}
