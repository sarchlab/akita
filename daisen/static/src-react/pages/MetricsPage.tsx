import { useCallback, useEffect, useMemo, useState } from "react";
import { useComponentNames } from "../hooks/useComponentNames";
import { useSegments } from "../hooks/useSegments";
import { useTraceData } from "../hooks/useTraceData";
import type { ComponentInfo } from "../hooks/useCompInfo";
import LineChart from "../components/charts/LineChart";
import type { DataPoint } from "../components/charts/LineChart";

const METRICS = [
  { key: "ReqInCount", label: "Incoming Request Rate" },
  { key: "ReqCompleteCount", label: "Request Complete Rate" },
  { key: "AvgLatency", label: "Average Latency" },
  { key: "BufferPressure", label: "Buffer Pressure" },
  { key: "ConcurrentTask", label: "Concurrent Tasks" },
  { key: "PendingReqOut", label: "Pending Outgoing Requests" },
] as const;

const COLORS = ["#2c7bb6", "#d7191c"];
const MAX_SELECTED = 16;
const NUM_DOTS = 40;

function toDataPoints(info: ComponentInfo | null): DataPoint[] {
  if (!info?.data) return [];
  return info.data.map((d) => ({ x: d.time, y: d.value }));
}

/**
 * MetricsPage — multi-component, dual-metric synchronized chart grid.
 *
 * Users select up to 16 components and 2 metrics. Each component gets
 * a chart showing both selected metrics overlaid, all sharing the same
 * time range.
 */
export default function MetricsPage() {
  const { names, loading: namesLoading } = useComponentNames();
  const { data: segData } = useSegments();
  const segments = segData?.segments ?? [];
  const simQuery = useMemo(() => ({ kind: "Simulation" }), []);
  const { tasks: simTasks } = useTraceData(simQuery);

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [metric1, setMetric1] = useState<string>(METRICS[0].key);
  const [metric2, setMetric2] = useState<string>(METRICS[1].key);
  const [filter, setFilter] = useState("");

  // Time range
  const [startTime, setStartTime] = useState(0);
  const [endTime, setEndTime] = useState(0);

  useEffect(() => {
    if (simTasks.length > 0) {
      setStartTime(simTasks[0].start_time);
      setEndTime(simTasks[0].end_time);
    } else if (segments.length > 0) {
      const minT = Math.min(...segments.map((s) => s.start_time));
      const maxT = Math.max(...segments.map((s) => s.end_time));
      setStartTime(minT);
      setEndTime(maxT);
    }
  }, [simTasks, segments]);

  // Chart data: fetch for each selected component × each metric
  const [chartData, setChartData] = useState<
    Record<string, Record<string, ComponentInfo | null>>
  >({});

  const fetchMetric = useCallback(
    (comp: string, metric: string, controller: AbortController) => {
      if (startTime >= endTime) return;

      const params = new URLSearchParams();
      params.set("where", comp);
      params.set("info_type", metric);
      params.set("start_time", String(startTime));
      params.set("end_time", String(endTime));
      params.set("num_dots", String(NUM_DOTS));

      fetch(`/api/compinfo?${params.toString()}`, {
        signal: controller.signal,
      })
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.json();
        })
        .then((json: ComponentInfo) => {
          setChartData((prev) => ({
            ...prev,
            [comp]: { ...prev[comp], [metric]: json },
          }));
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
        });
    },
    [startTime, endTime],
  );

  useEffect(() => {
    const controller = new AbortController();
    const metrics = [metric1, metric2];

    for (const comp of selected) {
      for (const m of metrics) {
        fetchMetric(comp, m, controller);
      }
    }

    return () => controller.abort();
  }, [selected, metric1, metric2, fetchMetric]);

  const toggleComponent = useCallback((name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else if (next.size < MAX_SELECTED) {
        next.add(name);
      }
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setSelected(new Set(filteredNames.slice(0, MAX_SELECTED)));
  }, [names, filter]);

  const selectNone = useCallback(() => {
    setSelected(new Set());
  }, []);

  const filteredNames = useMemo(() => {
    if (!filter) return names;
    try {
      const re = new RegExp(filter, "i");
      return names.filter((n) => re.test(n));
    } catch {
      const lower = filter.toLowerCase();
      return names.filter((n) => n.toLowerCase().includes(lower));
    }
  }, [names, filter]);

  const selectedArr = useMemo(
    () => Array.from(selected).sort(),
    [selected],
  );

  const m1Label =
    METRICS.find((m) => m.key === metric1)?.label ?? metric1;
  const m2Label =
    METRICS.find((m) => m.key === metric2)?.label ?? metric2;

  return (
    <div className="container-fluid py-2">
      <div className="row">
        {/* Left sidebar: component selector */}
        <div className="col-lg-2 col-md-3 mb-3">
          <h6>Components</h6>

          <input
            type="text"
            className="form-control form-control-sm mb-1"
            placeholder="Filter (regex)…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />

          <div className="mb-1 d-flex gap-1">
            <button
              className="btn btn-outline-secondary btn-sm"
              onClick={selectAll}
            >
              All
            </button>
            <button
              className="btn btn-outline-secondary btn-sm"
              onClick={selectNone}
            >
              None
            </button>
            <small className="text-muted align-self-center ms-1">
              {selected.size}/{MAX_SELECTED}
            </small>
          </div>

          {namesLoading ? (
            <div className="text-muted small">Loading…</div>
          ) : (
            <div
              style={{
                maxHeight: "calc(100vh - 250px)",
                overflowY: "auto",
                fontSize: 13,
              }}
            >
              {filteredNames.map((name) => (
                <div key={name} className="form-check">
                  <input
                    className="form-check-input"
                    type="checkbox"
                    id={`comp-${name}`}
                    checked={selected.has(name)}
                    onChange={() => toggleComponent(name)}
                    disabled={
                      !selected.has(name) && selected.size >= MAX_SELECTED
                    }
                  />
                  <label
                    className="form-check-label text-truncate d-block"
                    htmlFor={`comp-${name}`}
                    style={{ maxWidth: 200 }}
                  >
                    {name}
                  </label>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Main area: metric selectors + chart grid */}
        <div className="col-lg-10 col-md-9">
          {/* Metric selectors */}
          <div className="d-flex gap-3 mb-2 align-items-center flex-wrap">
            <div className="d-flex align-items-center gap-1">
              <span
                style={{
                  width: 12,
                  height: 12,
                  background: COLORS[0],
                  display: "inline-block",
                  borderRadius: 2,
                }}
              />
              <select
                className="form-select form-select-sm"
                style={{ width: "auto" }}
                value={metric1}
                onChange={(e) => setMetric1(e.target.value)}
              >
                {METRICS.map((m) => (
                  <option key={m.key} value={m.key}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>
            <div className="d-flex align-items-center gap-1">
              <span
                style={{
                  width: 12,
                  height: 12,
                  background: COLORS[1],
                  display: "inline-block",
                  borderRadius: 2,
                }}
              />
              <select
                className="form-select form-select-sm"
                style={{ width: "auto" }}
                value={metric2}
                onChange={(e) => setMetric2(e.target.value)}
              >
                {METRICS.map((m) => (
                  <option key={m.key} value={m.key}>
                    {m.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Legend */}
          {selectedArr.length > 0 && (
            <div className="text-muted small mb-2">
              <span style={{ color: COLORS[0] }}>{m1Label}</span>
              {" / "}
              <span style={{ color: COLORS[1] }}>{m2Label}</span>
              {" — "}
              {selectedArr.length} component
              {selectedArr.length !== 1 ? "s" : ""}
            </div>
          )}

          {/* Chart grid */}
          {selectedArr.length === 0 ? (
            <div className="text-muted mt-4">
              Select components from the left to view metrics.
            </div>
          ) : (
            <div
              className="d-flex flex-wrap gap-2"
              style={{ maxHeight: "calc(100vh - 180px)", overflowY: "auto" }}
            >
              {selectedArr.map((comp) => {
                const d1 = toDataPoints(
                  chartData[comp]?.[metric1] ?? null,
                );
                const d2 = toDataPoints(
                  chartData[comp]?.[metric2] ?? null,
                );
                return (
                  <div
                    key={comp}
                    className="border rounded p-1"
                    style={{ flex: "1 1 300px", maxWidth: 500 }}
                  >
                    <DualLineChart
                      title={comp}
                      data1={d1}
                      data2={d2}
                      color1={COLORS[0]}
                      color2={COLORS[1]}
                      startTime={startTime}
                      endTime={endTime}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

/* ── DualLineChart: two overlaid series with shared x-axis ──────── */

import * as d3 from "d3";
import { useRef } from "react";

function DualLineChart({
  title,
  data1,
  data2,
  color1,
  color2,
  startTime,
  endTime,
}: {
  title: string;
  data1: DataPoint[];
  data2: DataPoint[];
  color1: string;
  color2: string;
  startTime: number;
  endTime: number;
}) {
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const sel = d3.select(svg);
    sel.selectAll("*").remove();

    const w = svg.clientWidth || 300;
    const h = svg.clientHeight || 160;
    const margin = { top: 22, right: 40, bottom: 24, left: 40 };
    const innerW = w - margin.left - margin.right;
    const innerH = h - margin.top - margin.bottom;

    const xScale = d3
      .scaleLinear()
      .domain([startTime, endTime])
      .range([0, innerW]);

    const y1Max = d3.max(data1, (d) => d.y) ?? 1;
    const y2Max = d3.max(data2, (d) => d.y) ?? 1;

    const yScale1 = d3
      .scaleLinear()
      .domain([0, y1Max || 1])
      .nice()
      .range([innerH, 0]);

    const yScale2 = d3
      .scaleLinear()
      .domain([0, y2Max || 1])
      .nice()
      .range([innerH, 0]);

    const g = sel
      .append("g")
      .attr("transform", `translate(${margin.left},${margin.top})`);

    // Grid (from left axis)
    yScale1.ticks(4).forEach((tv) => {
      g.append("line")
        .attr("x1", 0)
        .attr("x2", innerW)
        .attr("y1", yScale1(tv) ?? 0)
        .attr("y2", yScale1(tv) ?? 0)
        .attr("stroke", "#eee")
        .attr("stroke-dasharray", "2,2");
    });

    // X axis
    g.append("g")
      .attr("transform", `translate(0,${innerH})`)
      .call(d3.axisBottom(xScale).ticks(3, ".3s"))
      .selectAll("text")
      .style("font-size", "9px");

    // Left Y axis (metric 1)
    const leftAxis = g.append("g")
      .call(d3.axisLeft(yScale1).ticks(4, ".2s"));
    leftAxis.selectAll("text").style("font-size", "9px").style("fill", color1);
    leftAxis.selectAll("line").style("stroke", color1);
    leftAxis.select(".domain").style("stroke", color1);

    // Right Y axis (metric 2)
    const rightAxis = g.append("g")
      .attr("transform", `translate(${innerW},0)`)
      .call(d3.axisRight(yScale2).ticks(4, ".2s"));
    rightAxis.selectAll("text").style("font-size", "9px").style("fill", color2);
    rightAxis.selectAll("line").style("stroke", color2);
    rightAxis.select(".domain").style("stroke", color2);

    // Draw series
    if (data1.length > 0) {
      const line1 = d3
        .line<DataPoint>()
        .x((d) => xScale(d.x) ?? 0)
        .y((d) => yScale1(d.y) ?? 0)
        .curve(d3.curveCatmullRom.alpha(0.5));

      g.append("path")
        .datum(data1)
        .attr("fill", "none")
        .attr("stroke", color1)
        .attr("stroke-width", 1.5)
        .attr("d", line1);
    }

    if (data2.length > 0) {
      const line2 = d3
        .line<DataPoint>()
        .x((d) => xScale(d.x) ?? 0)
        .y((d) => yScale2(d.y) ?? 0)
        .curve(d3.curveCatmullRom.alpha(0.5));

      g.append("path")
        .datum(data2)
        .attr("fill", "none")
        .attr("stroke", color2)
        .attr("stroke-width", 1.5)
        .attr("d", line2);
    }

    // Title
    sel
      .append("text")
      .attr("x", w / 2)
      .attr("y", 14)
      .attr("text-anchor", "middle")
      .attr("font-size", "11px")
      .attr("font-weight", "600")
      .text(title);
  }, [title, data1, data2, color1, color2, startTime, endTime]);

  return (
    <svg
      ref={svgRef}
      style={{ width: "100%", height: 160, display: "block" }}
    />
  );
}
