import { useRef, useEffect } from "react";
import * as d3 from "d3";

export interface DataPoint {
  x: number;
  y: number;
}

export interface LineChartProps {
  /** Array of {x, y} data points. */
  data: DataPoint[];
  /** Width of the SVG (default 600). */
  width?: number;
  /** Height of the SVG (default 200). */
  height?: number;
  /** X-axis label. */
  xLabel?: string;
  /** Y-axis label. */
  yLabel?: string;
  /** Line/dot color (default "#2c7bb6"). */
  color?: string;
  /** Chart title displayed at the top. */
  title?: string;
}

const MARGIN = { top: 30, right: 20, bottom: 40, left: 60 };

/** Safe wrapper around a D3 scale call — always returns a number. */
function sx(scale: d3.ScaleLinear<number, number>, v: number): number {
  return (scale(v) as number | undefined) ?? 0;
}

/**
 * A reusable SVG line chart rendered with D3 scales and React DOM.
 * Uses useRef + useEffect for D3 rendering while React owns the SVG element.
 */
export default function LineChart({
  data,
  width = 600,
  height = 200,
  xLabel = "",
  yLabel = "",
  color = "#2c7bb6",
  title = "",
}: LineChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const sel = d3.select(svg);
    sel.selectAll("*").remove();

    const innerW = width - MARGIN.left - MARGIN.right;
    const innerH = height - MARGIN.top - MARGIN.bottom;

    // Scales
    const xExtent = d3.extent(data, (d) => d.x) as [number, number];
    const yMax = d3.max(data, (d) => d.y) ?? 0;

    const xScale = d3
      .scaleLinear()
      .domain(xExtent[0] !== undefined ? xExtent : [0, 1])
      .range([0, innerW]);

    const yScale = d3
      .scaleLinear()
      .domain([0, yMax || 1])
      .nice()
      .range([innerH, 0]);

    // Container group
    const g = sel
      .append("g")
      .attr("transform", `translate(${MARGIN.left},${MARGIN.top})`);

    // Grid lines
    const yTicks = yScale.ticks(5);
    yTicks.forEach((tv) => {
      g.append("line")
        .attr("x1", 0)
        .attr("x2", innerW)
        .attr("y1", sx(yScale, tv))
        .attr("y2", sx(yScale, tv))
        .attr("stroke", "#e0e0e0")
        .attr("stroke-dasharray", "3,3");
    });

    // X axis
    const xAxis = d3.axisBottom(xScale).ticks(6, "s");
    g.append("g")
      .attr("transform", `translate(0,${innerH})`)
      .call(xAxis);

    // Y axis
    const yAxis = d3.axisLeft(yScale).ticks(5, ".2s");
    g.append("g").call(yAxis);

    // Line
    if (data.length > 0) {
      const line = d3
        .line<DataPoint>()
        .x((d) => sx(xScale, d.x))
        .y((d) => sx(yScale, d.y))
        .curve(d3.curveCatmullRom.alpha(0.5));

      g.append("path")
        .datum(data)
        .attr("fill", "none")
        .attr("stroke", color)
        .attr("stroke-width", 2)
        .attr("d", line);

      // Dots
      g.selectAll("circle")
        .data(data)
        .enter()
        .append("circle")
        .attr("cx", (d) => sx(xScale, d.x))
        .attr("cy", (d) => sx(yScale, d.y))
        .attr("r", 2.5)
        .attr("fill", color);
    }

    // Title
    if (title) {
      sel
        .append("text")
        .attr("x", width / 2)
        .attr("y", 16)
        .attr("text-anchor", "middle")
        .attr("font-size", "14px")
        .attr("font-weight", "600")
        .text(title);
    }

    // X label
    if (xLabel) {
      sel
        .append("text")
        .attr("x", width / 2)
        .attr("y", height - 4)
        .attr("text-anchor", "middle")
        .attr("font-size", "12px")
        .attr("fill", "#555")
        .text(xLabel);
    }

    // Y label
    if (yLabel) {
      sel
        .append("text")
        .attr("transform", `translate(14,${height / 2}) rotate(-90)`)
        .attr("text-anchor", "middle")
        .attr("font-size", "12px")
        .attr("fill", "#555")
        .text(yLabel);
    }
  }, [data, width, height, xLabel, yLabel, color, title]);

  return (
    <svg
      ref={svgRef}
      width={width}
      height={height}
      style={{ display: "block", maxWidth: "100%" }}
    />
  );
}
