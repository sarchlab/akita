import WidgetCard from "./WidgetCard";
import { useBlocked } from "../hooks/useBlocked";
import { formatVirtualTime } from "../lib/time";
import { cn } from "../lib/utils";

const TOP_COLOR = "#f97316"; // accent orange — the two most blocked
const REST_COLOR = "#94a3b8";

interface BlockedComponentsWidgetProps {
  expandHref?: string;
}

// BlockedComponentsWidget ranks components by the time their tasks spent blocked
// (waiting on a hardware resource, the network, a translation, etc.), with the
// two most blocked highlighted.
export default function BlockedComponentsWidget({
  expandHref,
}: BlockedComponentsWidgetProps) {
  const { data, loading, error } = useBlocked();
  const ranked = (data ?? []).filter((c) => c.blocked_time > 0);
  const max = ranked.length > 0 ? ranked[0].blocked_time : 1;

  return (
    <WidgetCard
      title="Most blocked components"
      expandHref={expandHref}
      contentClassName="overflow-auto p-3"
    >
      {loading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : error ? (
        <div className="text-sm text-destructive">{error}</div>
      ) : ranked.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No blocking recorded in this trace.
        </div>
      ) : (
        <ul className="flex flex-col gap-2.5">
          {ranked.map((c, i) => {
            const top = i < 2;
            const pct = Math.max(2, (c.blocked_time / max) * 100);
            return (
              <li key={c.component} className="flex flex-col gap-1">
                <div className="flex items-baseline justify-between gap-2 text-xs">
                  <span
                    className={cn(
                      "truncate",
                      top ? "font-semibold text-foreground" : "text-muted-foreground",
                    )}
                  >
                    {i + 1}. {c.component}
                  </span>
                  <span className="shrink-0 tabular-nums text-muted-foreground">
                    {formatVirtualTime(c.blocked_time)}
                  </span>
                </div>
                <div className="h-2 w-full overflow-hidden rounded bg-muted">
                  <div
                    className="h-full rounded"
                    style={{
                      width: `${pct}%`,
                      backgroundColor: top ? TOP_COLOR : REST_COLOR,
                    }}
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </WidgetCard>
  );
}
