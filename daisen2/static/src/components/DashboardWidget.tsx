import { Link } from "react-router-dom";
import { Maximize2, ExternalLink } from "lucide-react";
import { useCompInfo } from "../hooks/useCompInfo";
import type { Segment } from "../types/task";
import TimeSeriesChart from "./charts/TimeSeriesChart";
import { Card } from "./ui/card";
import { axisLabel } from "../utils/metrics";

interface DashboardWidgetProps {
  name: string;
  width: number;
  height: number;
  startTime: number;
  endTime: number;
  dataStartTime: number;
  dataEndTime: number;
  dataPending: boolean;
  primaryAxis: string;
  secondaryAxis: string;
  segments: Segment[];
  segmentsEnabled: boolean;
  onTimeRangeChange: (range: { startTime: number; endTime: number }) => void;
  // When provided, renders an "expand" control that asks the parent to show only
  // this widget (sets the dashboard's `widget` URL param).
  onFocus?: (name: string) => void;
  // When the chart sums a multi-facet subtree (an internal node), mark it so the
  // reader knows the curve is an aggregate, not a single location. facetCount is
  // the number of leaf facets summed, shown for context.
  aggregated?: boolean;
  facetCount?: number;
  // When set, render a compact legend under the chart naming the two metrics
  // (their labels can differ per chart, e.g. auto-selected in the components
  // widget). The dashboard grid leaves it off — its axis menus are the legend.
  showLegend?: boolean;
}

const HEADER_HEIGHT = 30;
const LEGEND_HEIGHT = 18;
const PRIMARY_COLOR = "#d7191c";
const SECONDARY_COLOR = "#2c7bb6";
const iconButton =
  "shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export default function DashboardWidget({
  name,
  width,
  height,
  startTime,
  endTime,
  dataStartTime,
  dataEndTime,
  dataPending,
  primaryAxis,
  secondaryAxis,
  segments,
  segmentsEnabled,
  onTimeRangeChange,
  onFocus,
  aggregated,
  facetCount,
  showLegend,
}: DashboardWidgetProps) {
  const primary = useCompInfo(name, primaryAxis, dataStartTime, dataEndTime);
  const secondary = useCompInfo(name, secondaryAxis, dataStartTime, dataEndTime);
  const hasActiveAxis = primaryAxis !== "-" || secondaryAxis !== "-";
  const dataUpdating = (dataPending && hasActiveAxis) || primary.loading || secondary.loading;
  const chartHeight = Math.max(70, height - HEADER_HEIGHT - (showLegend ? LEGEND_HEIGHT : 0));
  // Card border (2) + the chart row's px-1 (8); a couple px of slack avoids a
  // sub-pixel horizontal scrollbar while still filling the card.
  const chartWidth = Math.max(160, width - 12);

  const href = `/component?name=${encodeURIComponent(name)}&starttime=${startTime}&endtime=${endTime}`;

  return (
    <Card className="flex flex-col overflow-hidden border-slate-300 p-0" style={{ height }}>
      {/* Header: name + (aggregate marker / updating) on the left, actions right. */}
      <div className="flex h-[30px] shrink-0 items-center gap-1.5 border-b px-2">
        <span className="min-w-0 truncate text-sm font-semibold" title={name}>
          {name}
        </span>
        {aggregated ? (
          <span
            className="shrink-0 rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-primary"
            title={`Aggregated: summed over ${facetCount ?? "all"} leaf facets in the ${name} subtree`}
          >
            {facetCount ? `Σ ${facetCount} facets` : "aggregated"}
          </span>
        ) : null}
        {dataUpdating ? (
          <span
            className="daisen-widget-update-indicator"
            title={dataPending ? "Waiting to refresh chart data" : "Refreshing chart data"}
            aria-live="polite"
          >
            <span className="daisen-widget-update-spinner" aria-hidden="true" />
            Updating
          </span>
        ) : null}
        <span className="flex-1" />
        <Link to={href} className={iconButton} title="Open the detailed component view" aria-label={`Open ${name} detail view`}>
          <ExternalLink className="h-3.5 w-3.5" />
        </Link>
        {onFocus ? (
          <button
            type="button"
            className={iconButton}
            title="Expand this chart to fill the dashboard"
            aria-label={`Expand ${name}`}
            onClick={() => onFocus(name)}
          >
            <Maximize2 className="h-3.5 w-3.5" />
          </button>
        ) : null}
      </div>

      <div className="min-h-0 flex-1 px-1" data-widget-name={name}>
        <TimeSeriesChart
          width={chartWidth}
          height={chartHeight}
          startTime={startTime}
          endTime={endTime}
          segments={segments}
          segmentsEnabled={segmentsEnabled}
          onTimeRangeChange={onTimeRangeChange}
          series={[
            { info: primaryAxis === "-" ? null : primary.info, color: PRIMARY_COLOR, side: "left" },
            { info: secondaryAxis === "-" ? null : secondary.info, color: SECONDARY_COLOR, side: "right" },
          ]}
        />
      </div>

      {showLegend ? (
        <div className="flex h-[18px] shrink-0 items-center gap-3 overflow-hidden border-t px-2 text-[10px] leading-none text-muted-foreground">
          {primaryAxis !== "-" ? (
            <span className="flex min-w-0 items-center gap-1">
              <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: PRIMARY_COLOR }} />
              <span className="truncate">{axisLabel(primaryAxis)}</span>
            </span>
          ) : null}
          {secondaryAxis !== "-" ? (
            <span className="flex min-w-0 items-center gap-1">
              <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: SECONDARY_COLOR }} />
              <span className="truncate">{axisLabel(secondaryAxis)}</span>
            </span>
          ) : null}
        </div>
      ) : null}

      {(primary.error || secondary.error) && (
        <div className="truncate px-2 pb-1 text-xs text-destructive">{primary.error ?? secondary.error}</div>
      )}
    </Card>
  );
}
