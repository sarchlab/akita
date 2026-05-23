import { Link } from "react-router-dom";
import type { LucideIcon } from "lucide-react";
import { buttonVariants } from "./ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";
import { cn } from "../lib/utils";

interface DashboardNavigationWidgetProps {
  to: string;
  title: string;
  meta: string;
  icon: LucideIcon;
  height: number;
}

export default function DashboardNavigationWidget({
  to,
  title,
  meta,
  icon: Icon,
  height,
}: DashboardNavigationWidgetProps) {
  return (
    <Link to={to} className="block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
      <Card className="flex flex-col overflow-hidden rounded-md border-slate-400 shadow-none hover:border-primary" style={{ height }}>
        <CardHeader className="flex-row items-center gap-2 space-y-0 pb-2">
          <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Icon className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <CardTitle className="truncate text-sm">{title}</CardTitle>
            <div className="truncate text-xs text-muted-foreground">{meta}</div>
          </div>
        </CardHeader>
        <CardContent className="flex min-h-0 flex-1 flex-col justify-between">
          <div className="grid grid-cols-6 gap-1">
            {Array.from({ length: 18 }).map((_, index) => (
              <span
                key={index}
                className="h-2 rounded-sm bg-slate-200"
                style={{ opacity: 0.35 + ((index % 5) + 1) * 0.1 }}
              />
            ))}
          </div>
          <span className={cn(buttonVariants({ size: "sm" }), "mt-3 self-start")}>Open</span>
        </CardContent>
      </Card>
    </Link>
  );
}
