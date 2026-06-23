import { useMemo } from "react";
import WidgetCard from "./WidgetCard";
import DashboardWidget from "./DashboardWidget";
import { useComponents } from "../hooks/useComponents";
import { useComponentNames } from "../hooks/useComponentNames";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useSegments } from "../hooks/useSegments";
import { useElementSize } from "../hooks/useElementSize";
import { buildLocationTree, findNode, leafCount } from "../utils/locationTree";

const GAP = 8;
const COUNT = 4;

interface ComponentsWidgetProps {
  expandHref?: string;
}

// pickMetrics auto-selects the two metrics most informative for a component.
// Components that serve requests (have a req_in port) chart their incoming request
// rate; pure clients that only issue requests (e.g. the mem agent) have no incoming
// requests, so chart the response backlog they accumulate instead. Average latency
// is meaningful either way.
function pickMetrics(component: string, names: string[]): { primary: string; secondary: string } {
  const servesRequests = names.some((n) => n.startsWith(`${component}.`) && n.endsWith(".req_in"));
  return {
    primary: servesRequests ? "ReqInCount" : "ResponseBufferPressure",
    secondary: "AvgLatency",
  };
}

// ComponentsWidget shows the components that hold the most total task time
// (residency) — the busiest / most-contended components — in a 2x2 grid. Each
// chart's two metrics are auto-selected to suit that component (see pickMetrics)
// and named in a small per-chart legend. Enlarging the widget opens the dashboard.
export default function ComponentsWidget({ expandHref }: ComponentsWidgetProps) {
  const { data, loading, error } = useComponents();
  const { names } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segments } = useSegments();
  const { ref, size } = useElementSize<HTMLDivElement>();

  // The same location tree the dashboard builds, so a scope's facet count (leaf
  // locations summed into its chart) matches the dashboard's "Σ N facets" badge.
  const root = useMemo(() => buildLocationTree(names ?? []), [names]);

  const top = (data ?? []).filter((c) => c.task_time > 0).slice(0, COUNT);
  // 2x2 grid: half the width and half the height per chart (minus the gaps).
  const cellWidth = Math.max(160, (size.width - GAP) / 2);
  const cellHeight = Math.max(120, (size.height - GAP) / 2);

  return (
    <WidgetCard title="Components" expandHref={expandHref} contentClassName="p-2">
      {/* The ref must be on an always-mounted element so the ResizeObserver
          attaches on mount; otherwise the charts keep their default width. */}
      <div
        ref={ref}
        className="grid h-full min-h-0 grid-cols-2 content-start"
        style={{ gap: GAP }}
      >
        {loading ? (
          <div className="text-sm text-muted-foreground">Loading…</div>
        ) : error ? (
          <div className="text-sm text-destructive">{error}</div>
        ) : top.length === 0 ? (
          <div className="text-sm text-muted-foreground">No tasks recorded in this trace.</div>
        ) : (
          top.map((c) => {
            const node = findNode(root, c.component);
            const aggregated = !!node && node.children.length > 0;
            const { primary, secondary } = pickMetrics(c.component, names ?? []);
            return (
              <DashboardWidget
                key={c.component}
                name={c.component}
                width={cellWidth}
                height={cellHeight}
                startTime={startTime}
                endTime={endTime}
                dataStartTime={startTime}
                dataEndTime={endTime}
                dataPending={false}
                primaryAxis={primary}
                secondaryAxis={secondary}
                segments={segments?.segments ?? []}
                segmentsEnabled={segments?.enabled ?? false}
                onTimeRangeChange={() => {}}
                aggregated={aggregated}
                facetCount={aggregated && node ? leafCount(node) : undefined}
                showLegend
              />
            );
          })
        )}
      </div>
    </WidgetCard>
  );
}
