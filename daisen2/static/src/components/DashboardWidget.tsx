import { Link } from "react-router-dom";
import { useCompInfo } from "../hooks/useCompInfo";
import type { Segment } from "../types/task";
import TimeSeriesChart from "./charts/TimeSeriesChart";

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
}

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
}: DashboardWidgetProps) {
  const primary = useCompInfo(name, primaryAxis, dataStartTime, dataEndTime);
  const secondary = useCompInfo(name, secondaryAxis, dataStartTime, dataEndTime);
  const hasActiveAxis = primaryAxis !== "-" || secondaryAxis !== "-";
  const dataUpdating = (dataPending && hasActiveAxis) || primary.loading || secondary.loading;
  const chartHeight = Math.max(70, height - 28);
  const chartWidth = Math.max(160, width - 18);

  const href = `/component?name=${encodeURIComponent(name)}&starttime=${startTime}&endtime=${endTime}`;

  return (
    <Link className="daisen-widget block focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" style={{ height }} to={href}>
      <div className="daisen-widget-title">
        <span className="min-w-0 flex-1 truncate hover:text-primary" title={name}>
          {name}
        </span>
        {dataUpdating ? (
          <span
            className="daisen-widget-update-indicator"
            title={dataPending ? "Waiting to refresh chart data" : "Refreshing chart data"}
            aria-label={dataPending ? "Waiting to refresh chart data" : "Refreshing chart data"}
            aria-live="polite"
          >
            <span className="daisen-widget-update-spinner" aria-hidden="true" />
            Updating
          </span>
        ) : null}
      </div>
      <div data-widget-name={name}>
        <TimeSeriesChart
          width={chartWidth}
          height={chartHeight}
          startTime={startTime}
          endTime={endTime}
          segments={segments}
          segmentsEnabled={segmentsEnabled}
          onTimeRangeChange={onTimeRangeChange}
          series={[
            { info: primaryAxis === "-" ? null : primary.info, color: "#d7191c", side: "left" },
            { info: secondaryAxis === "-" ? null : secondary.info, color: "#2c7bb6", side: "right" },
          ]}
        />
      </div>
      {(primary.error || secondary.error) && (
        <div className="truncate text-xs text-destructive">{primary.error ?? secondary.error}</div>
      )}
    </Link>
  );
}
