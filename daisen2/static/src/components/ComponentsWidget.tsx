import { useMemo } from "react";
import WidgetCard from "./WidgetCard";
import DashboardWidget from "./DashboardWidget";
import { ComponentsOverviewHelp } from "./HelpTopics";
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

// pickMetrics auto-selects the two metrics most informative for a component, by the
// task kinds it actually produces:
//   - Request-fulfilling (caches, memory, translators — anything that records a
//     req_in) -> incoming request rate + average request latency.
//   - Executors (cores, traffic agents — they issue req_out but serve none) ->
//     concurrent task count + outstanding requests.
//   - Network components (switches/endpoints — flit/flit_e2e/msg_e2e, and no req
//     tasks) -> concurrent task count alone, since the request-based metrics would
//     all read zero.
// Keying on kinds (not location facets) correctly classifies a server that records
// req_in at its own location with no ".req_in" child, and lets a network component
// show a non-zero metric instead of empty request lines.
function pickMetrics(kinds: string[]): { primary: string; secondary: string } {
  const has = (k: string) => kinds.includes(k);
  if (has("req_in")) return { primary: "ReqInCount", secondary: "AvgLatency" };
  if (has("req_out")) return { primary: "ConcurrentTask", secondary: "PendingReqOut" };
  return { primary: "ConcurrentTask", secondary: "-" };
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
    <WidgetCard title="Components" expandHref={expandHref} info={<ComponentsOverviewHelp />} contentClassName="p-2">
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
            const { primary, secondary } = pickMetrics(c.kinds ?? []);
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
