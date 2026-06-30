import { useTopBlockingResources } from "../hooks/useTopBlockingResources";
import WidgetCard from "./WidgetCard";

interface BlockingResourcesWidgetProps {
  expandHref?: string;
  bare?: boolean;
}

// BlockingResourcesWidget is the index-page overview widget ranking the hardware
// resources that blocked tasks the most across the whole trace, by number of
// blocking events (hardware_resource milestones naming each resource). The
// distinct-task count is shown alongside as the breadth of impact.
export default function BlockingResourcesWidget({
  expandHref,
  bare,
}: BlockingResourcesWidgetProps) {
  // The enlarged page has room for a longer list.
  const { data, loading, error } = useTopBlockingResources("", bare ? 30 : 12);
  const resources = data?.resources ?? [];
  const max = resources.reduce((m, r) => Math.max(m, r.count), 0);

  return (
    <WidgetCard title="Top Blocking Resources" expandHref={expandHref} bare={bare}>
      {loading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : error ? (
        <div className="text-sm text-destructive">{error}</div>
      ) : resources.length === 0 ? (
        <div className="text-sm text-muted-foreground">
          No hardware-resource blocking recorded in this trace.
        </div>
      ) : (
        // Each row is a horizontal bar: a fill proportional to the count sits
        // behind the resource name and count, so the ranking reads at a glance and
        // the layout works at a third-width card or the enlarged page alike.
        <ol className="flex flex-col gap-1">
          {resources.map((resource, index) => (
            <li
              key={resource.what}
              className="relative flex items-center gap-2 overflow-hidden rounded px-2 py-1 text-xs"
              title={`${resource.count.toLocaleString()} blocking event${resource.count === 1 ? "" : "s"} across ${resource.task_count.toLocaleString()} task${resource.task_count === 1 ? "" : "s"}`}
            >
              <span
                className="absolute inset-y-0 left-0 rounded bg-primary/15"
                style={{ width: `${max > 0 ? (resource.count / max) * 100 : 0}%` }}
                aria-hidden="true"
              />
              <span className="relative w-5 shrink-0 text-right tabular-nums text-muted-foreground">
                {index + 1}
              </span>
              <span className="relative min-w-0 flex-1 truncate font-medium" title={resource.what}>
                {resource.what}
              </span>
              <span className="relative shrink-0 tabular-nums text-muted-foreground">
                {resource.count.toLocaleString()}
              </span>
            </li>
          ))}
        </ol>
      )}
    </WidgetCard>
  );
}
