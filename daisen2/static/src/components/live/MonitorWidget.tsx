import { useEffect, useRef, useState } from "react";
import * as d3 from "d3";
import { isContainerKind, VarKind } from "../../types/gotypes";

/** A single data point in the rolling window. */
interface DataPoint {
  time: number;
  value: number;
}

interface MonitorWidgetProps {
  component: string;
  field: string;
  onClose: () => void;
}

/** Max data points kept in the rolling window. */
const MAX_POINTS = 300;

/**
 * MonitorWidget — polls a component field every second and renders
 * a D3 bar chart with rolling 300-point history.
 */
export default function MonitorWidget({
  component,
  field,
  onClose,
}: MonitorWidgetProps) {
  const [data, setData] = useState<DataPoint[]>([]);
  const svgRef = useRef<SVGSVGElement | null>(null);

  /* Poll /api/field + /api/now every 1 s */
  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;

    const poll = () => {
      const req = {
        comp_name: component,
        field_name: field.startsWith(".") ? field.substring(1) : field,
      };

      Promise.all([
        fetch(`/api/field/${JSON.stringify(req)}`, { signal }),
        fetch("/api/now", { signal }),
      ])
        .then(([res1, res2]) => Promise.all([res1.json(), res2.json()]))
        .then(([fieldData, nowData]) => {
          const entry = fieldData["dict"]?.[fieldData["r"]];
          let value = 0;
          if (entry) {
            value = isContainerKind(entry["k"] as VarKind)
              ? entry["l"]
              : entry["v"];
          }

          setData((prev) => {
            const next = [...prev, { time: nowData["now"], value }];
            return next.length > MAX_POINTS ? next.slice(-MAX_POINTS) : next;
          });
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
        });
    };

    poll();
    const timer = setInterval(poll, 1000);
    return () => {
      controller.abort();
      clearInterval(timer);
    };
  }, [component, field]);

  /* Render D3 bar chart whenever data changes */
  useEffect(() => {
    const svgEl = svgRef.current;
    if (!svgEl || data.length === 0) return;

    const svg = d3.select(svgEl);
    const canvasWidth = svgEl.clientWidth || 300;
    const canvasHeight = svgEl.clientHeight || 120;

    const padding = 8;
    const yAxisWidth = 30;
    const xAxisHeight = 18;
    const contentWidth = canvasWidth - yAxisWidth - padding * 2;
    const contentHeight = canvasHeight - xAxisHeight - padding * 2;

    const xMin = d3.min(data, (d) => d.time) ?? 0;
    const xMax = d3.max(data, (d) => d.time) ?? 1;
    const yMax = d3.max(data, (d) => d.value) ?? 1;

    const xScale = d3.scaleLinear().domain([xMin, xMax]).range([0, contentWidth]);
    const yScale = d3.scaleLinear().domain([0, yMax]).range([contentHeight, 0]);
    const x = (v: number) => xScale(v) ?? 0;
    const y = (v: number) => yScale(v) ?? 0;

    // Axes
    const xAxisG = svg
      .select<SVGGElement>(".x-axis")
      .attr(
        "transform",
        `translate(${yAxisWidth + padding}, ${contentHeight + padding})`,
      );
    const yAxisG = svg
      .select<SVGGElement>(".y-axis")
      .attr("transform", `translate(${yAxisWidth + padding}, ${padding})`);

    xAxisG.call(d3.axisBottom(xScale).ticks(4));
    yAxisG.call(d3.axisLeft(yScale).ticks(4));

    // Bars
    const barGroup = svg.select(".bar-group");
    const bars = barGroup
      .selectAll<SVGRectElement, DataPoint>("rect")
      .data(data, (d: DataPoint) => d.time);

    const barWidth = Math.max(contentWidth / data.length, 1);

    const enterBars = bars
      .enter()
      .append("rect")
      .attr("x", (d: DataPoint) => x(d.time) + padding + yAxisWidth)
      .attr("y", padding + contentHeight)
      .attr("width", barWidth)
      .attr("height", 0)
      .attr("fill", "#666");

    bars
      .merge(enterBars)
      .transition()
      .duration(200)
      .attr("x", (d: DataPoint) => x(d.time) + padding + yAxisWidth)
      .attr("y", (d: DataPoint) => padding + y(d.value))
      .attr("width", barWidth)
      .attr("height", (d: DataPoint) => contentHeight - y(d.value))
      .attr("fill", "#666");

    bars.exit().remove();
  }, [data]);

  if (data.length === 0) return null;

  return (
    <div
      className="border rounded p-2"
      style={{ flex: "1 1 0", minWidth: 200 }}
    >
      <div className="d-flex justify-content-between align-items-center mb-1">
        <small className="text-truncate fw-bold">
          {component}
          {field}
        </small>
        <button
          className="btn-close btn-close-sm"
          aria-label="Close"
          onClick={onClose}
          style={{ fontSize: 10 }}
        />
      </div>
      <svg ref={svgRef} width="100%" height={120}>
        <g className="x-axis" />
        <g className="y-axis" />
        <g className="bar-group" />
      </svg>
    </div>
  );
}
