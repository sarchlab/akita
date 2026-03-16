import { useEffect, useMemo, useState } from "react";
import { useSearchParams, Link } from "react-router";
import LineChart from "../components/charts/LineChart";
import type { DataPoint } from "../components/charts/LineChart";
import { useCompInfo, type ComponentInfo } from "../hooks/useCompInfo";
import { useSegments } from "../hooks/useSegments";
import { useTraceData } from "../hooks/useTraceData";
import { smartString } from "../utils/smartValue";

/** Metric definitions matching the Go backend info_type values. */
const METRICS = [
  {
    key: "ReqInCount",
    label: "Incoming Request Rate",
    color: "#d7191c",
    yLabel: "Rate",
  },
  {
    key: "ReqCompleteCount",
    label: "Request Complete Rate",
    color: "#fdae61",
    yLabel: "Rate",
  },
  {
    key: "AvgLatency",
    label: "Average Latency",
    color: "#2c7bb6",
    yLabel: "Latency (s)",
  },
  {
    key: "BufferPressure",
    label: "Buffer Pressure",
    color: "#abd9e9",
    yLabel: "Pressure",
  },
  {
    key: "ConcurrentTask",
    label: "Concurrent Tasks",
    color: "#1a9641",
    yLabel: "Count",
  },
] as const;

const NUM_DOTS = 40;

/** Convert ComponentInfo data to LineChart DataPoints. */
function toDataPoints(info: ComponentInfo | null): DataPoint[] {
  if (!info?.data) return [];
  return info.data.map((d) => ({ x: d.time, y: d.value }));
}

/**
 * Component analytics page.
 *
 * Shows multiple line charts for a given component:
 * - Incoming Request Rate
 * - Request Complete Rate
 * - Average Latency
 * - Buffer Pressure
 * - Concurrent Tasks
 */
export default function ComponentPage() {
  const [searchParams] = useSearchParams();
  const compName = searchParams.get("name") ?? "";

  // Try to get time range from URL or from simulation trace
  const urlStart = searchParams.get("starttime");
  const urlEnd = searchParams.get("endtime");

  const simQuery = useMemo(() => ({ kind: "Simulation" }), []);
  const { tasks: simTasks } = useTraceData(simQuery);
  const { data: segData } = useSegments();
  const segments = segData?.segments ?? [];

  // Determine time range
  const [startTime, setStartTime] = useState(0);
  const [endTime, setEndTime] = useState(0);

  useEffect(() => {
    if (urlStart && urlEnd) {
      setStartTime(parseFloat(urlStart));
      setEndTime(parseFloat(urlEnd));
    } else if (simTasks.length > 0) {
      setStartTime(simTasks[0].start_time);
      setEndTime(simTasks[0].end_time);
    }
  }, [urlStart, urlEnd, simTasks]);

  // Chart container width state for responsiveness
  const [chartWidth, setChartWidth] = useState(560);

  useEffect(() => {
    const updateWidth = () => {
      // Leave margin for the page padding and card borders
      const available = Math.min(window.innerWidth - 80, 900);
      setChartWidth(Math.max(300, available));
    };
    updateWidth();
    window.addEventListener("resize", updateWidth);
    return () => window.removeEventListener("resize", updateWidth);
  }, []);

  // Fetch all metrics
  const reqIn = useCompInfo(compName, "ReqInCount", startTime, endTime, NUM_DOTS);
  const reqComplete = useCompInfo(compName, "ReqCompleteCount", startTime, endTime, NUM_DOTS);
  const avgLatency = useCompInfo(compName, "AvgLatency", startTime, endTime, NUM_DOTS);
  const bufPressure = useCompInfo(compName, "BufferPressure", startTime, endTime, NUM_DOTS);
  const concurrent = useCompInfo(compName, "ConcurrentTask", startTime, endTime, NUM_DOTS);

  const metricData: { info: ComponentInfo | null; loading: boolean; error: string | null }[] = [
    reqIn,
    reqComplete,
    avgLatency,
    bufPressure,
    concurrent,
  ];

  if (!compName) {
    return (
      <div className="container py-4">
        <h4>Component Analytics</h4>
        <p className="text-muted">
          No component selected. Go to the{" "}
          <Link to="/">Dashboard</Link> to pick a component.
        </p>
      </div>
    );
  }

  const anyLoading = metricData.some((m) => m.loading);

  return (
    <div className="container-fluid py-2">
      {/* Header */}
      <div className="d-flex align-items-center gap-3 mb-3 flex-wrap">
        <Link to="/" className="btn btn-sm btn-outline-secondary">
          ← Dashboard
        </Link>
        <h4 className="mb-0">{compName}</h4>
        {anyLoading && (
          <span className="spinner-border spinner-border-sm text-primary" />
        )}
      </div>

      {/* Time range info */}
      {startTime < endTime && (
        <p className="text-muted small mb-3">
          Time range: {smartString(startTime)} → {smartString(endTime)}
          {segments.length > 0 && (
            <span className="ms-3">({segments.length} traced segments)</span>
          )}
        </p>
      )}

      {/* Charts */}
      <div className="row g-3">
        {METRICS.map((metric, idx) => {
          const { info, loading, error } = metricData[idx];
          const points = toDataPoints(info);

          return (
            <div key={metric.key} className="col-12">
              <div className="card">
                <div className="card-body p-2">
                  {loading && (
                    <div className="text-center py-4">
                      <div className="spinner-border spinner-border-sm" />
                      <span className="ms-2 text-muted">
                        Loading {metric.label}…
                      </span>
                    </div>
                  )}
                  {error && (
                    <div className="alert alert-danger py-1 mb-0">
                      {metric.label}: {error}
                    </div>
                  )}
                  {!loading && !error && (
                    <LineChart
                      data={points}
                      width={chartWidth}
                      height={200}
                      title={metric.label}
                      xLabel="Time (s)"
                      yLabel={metric.yLabel}
                      color={metric.color}
                    />
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
