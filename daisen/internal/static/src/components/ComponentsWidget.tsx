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
// Each sub-chart needs at least this much room to stay legible; the grid fits as
// many whole MIN_W × MIN_H cells as the measured area allows. MIN_W is generous so
// a narrow widget collapses to a single taller column rather than a cramped 2-wide
// grid with a half-empty bottom.
const MIN_W = 260;
const MIN_H = 140;
// A safety cap so a very large area does not mount an unreasonable number of
// charts (each fetches two metric series).
const MAX_CHARTS = 24;

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
// (residency) — the busiest / most-contended components — in a grid whose chart
// count follows the widget's measured size: as many whole MIN_W × MIN_H cells as
// fit. Each chart's two metrics are auto-selected to suit that component (see
// pickMetrics) and named in a small per-chart legend. Enlarging opens the dashboard.
export default function ComponentsWidget({ expandHref }: ComponentsWidgetProps) {
  const { data, loading, error } = useComponents();
  const { names } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segments } = useSegments();
  const { ref, size } = useElementSize<HTMLDivElement>();

  // The same location tree the dashboard builds, so a scope's facet count (leaf
  // locations summed into its chart) matches the dashboard's "Σ N facets" badge.
  const root = useMemo(() => buildLocationTree(names ?? []), [names]);

  // How many whole MIN_W × MIN_H cells the measured area can hold, and how many
  // charts we actually have to show (the busiest components, capped).
  const cols = Math.max(1, Math.floor((size.width + GAP) / (MIN_W + GAP)));
  const maxRows = Math.max(1, Math.floor((size.height + GAP) / (MIN_H + GAP)));
  const available = (data ?? []).filter((c) => c.task_time > 0);
  const count = Math.min(available.length, cols * maxRows, MAX_CHARTS);
  const top = available.slice(0, count);

  // Lay the charts out using only as many columns and rows as they actually fill,
  // then divide the whole area evenly among them — so the grid always fills the
  // full space (no empty band below) while each cell stays at/above the minimum.
  const usedCols = Math.max(1, Math.min(cols, count || 1));
  const usedRows = Math.max(1, Math.ceil((count || 1) / usedCols));
  const cellWidth = (size.width - (usedCols - 1) * GAP) / usedCols;
  const cellHeight = (size.height - (usedRows - 1) * GAP) / usedRows;

  return (
    <WidgetCard title="Components" expandHref={expandHref} info={<ComponentsOverviewHelp />} contentClassName="p-2">
      {/* The ref must be on an always-mounted element so the ResizeObserver
          attaches on mount; otherwise the charts keep their default width. */}
      <div
        ref={ref}
        className="grid h-full min-h-0 content-start"
        style={{ gap: GAP, gridTemplateColumns: `repeat(${usedCols}, minmax(0, 1fr))` }}
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
