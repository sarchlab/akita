import { t } from "../lib/i18n";
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Maximize2 } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { cn } from "../lib/utils";

/** Daisen's overview widget frame: a titled card, optionally linking to its full page. */
export default function WidgetCard({ title, expandHref, headerRight, contentClassName, className, children }: {
  title: ReactNode; expandHref?: string; headerRight?: ReactNode; contentClassName?: string; className?: string; children: ReactNode;
}) {
  return (
    <Card className={cn("flex min-h-0 min-w-0 flex-col", className)}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <div className="flex min-w-0 items-center gap-1.5"><CardTitle className="truncate">{title}</CardTitle></div>
        <div className="flex items-center gap-3">
          {headerRight}
          {expandHref ? (
            <Link to={expandHref} title={t("common.openFull")} aria-label={t("common.openFull")} className="text-muted-foreground transition-colors hover:text-foreground">
              <Maximize2 className="h-4 w-4" />
            </Link>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className={cn("min-h-0 flex-1", contentClassName)}>{children}</CardContent>
    </Card>
  );
}
