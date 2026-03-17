import { useCallback, useEffect, useRef, useState } from "react";
import { useMode } from "../../hooks/useMode";
import * as d3 from "d3";
import {
  PProfData,
  PProfEdge,
  PProfNetwork,
  PProfNode,
  pprofDataToNetwork,
} from "../../types/pprof";

/** Shape returned by GET /api/resource */
interface ResourceUsage {
  cpu_percent: number;
  memory_size: number;
}

function formatCPUPercent(percent: number): string {
  return percent.toFixed(0) + "%";
}

function formatBytes(bytes: number): string {
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  if (bytes === 0) return "0";
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return Math.round(bytes / Math.pow(1024, i)) + " " + sizes[i];
}

/**
 * ResourceMonitor — polls /api/resource every second for CPU & memory,
 * with a Profile button that fetches /api/profile and renders a D3 pprof
 * graph below.
 */
export default function ResourceMonitor() {
  const { mode } = useMode();
  const [resource, setResource] = useState<ResourceUsage | null>(null);
  const [profiling, setProfiling] = useState(false);
  const [network, setNetwork] = useState<PProfNetwork | null>(null);

  /* Poll /api/resource every 1 s */
  useEffect(() => {
    if (mode !== "live") return;
    const controller = new AbortController();

    const poll = () => {
      fetch("/api/resource", { signal: controller.signal })
        .then((res) => res.json())
        .then((data: ResourceUsage) => setResource(data))
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
  }, [mode]);

  const handleProfile = useCallback(() => {
    setProfiling(true);
    fetch("/api/profile")
      .then((res) => res.json())
      .then((data: PProfData) => {
        const net = pprofDataToNetwork(data);
        setNetwork(net);
      })
      .catch(() => {
        /* ignore */
      })
      .finally(() => setProfiling(false));
  }, []);

  if (mode !== "live") return null;

  return (
    <div>
      <h6 className="mb-2">Resource Monitor</h6>

      {/* CPU / Memory readout */}
      <div className="d-flex align-items-center gap-3 mb-2">
        <span>
          <strong>CPU:</strong>{" "}
          {resource ? formatCPUPercent(resource.cpu_percent) : "—"}
        </span>
        <span>
          <strong>Memory:</strong>{" "}
          {resource ? formatBytes(resource.memory_size) : "—"}
        </span>
        <button
          className="btn btn-sm btn-outline-secondary"
          onClick={handleProfile}
          disabled={profiling}
        >
          {profiling ? "Profiling…" : "Profile"}
        </button>
      </div>

      {/* PProfGraph */}
      {network && <PProfGraph network={network} />}
    </div>
  );
}

/* ── PProfGraph (D3 SVG) ──────────────────────────────────── */

function PProfGraph({ network }: { network: PProfNetwork }) {
  const svgRef = useRef<SVGSVGElement | null>(null);
  const tooltipRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;

    const d3SVG = d3.select<SVGSVGElement, unknown>(svg);
    d3SVG.selectAll("*").remove();

    // Arrow marker
    d3SVG
      .append("defs")
      .append("marker")
      .attr("id", "arrowhead")
      .attr("viewBox", "0 -66 200 200")
      .attr("refX", 5)
      .attr("refY", 2)
      .attr("markerWidth", 18)
      .attr("markerHeight", 12)
      .attr("orient", "auto")
      .attr("markerUnits", "userSpaceOnUse")
      .append("path")
      .attr("d", "M0,-66 L0,66 L200,0");

    const nodeGroup = d3SVG.append("g").attr("class", "pprof-nodes");
    const edgeGroup = d3SVG.append("g").attr("class", "pprof-edges");

    // Zoom / pan — D3 v7 passes event as first arg; v5 types expect datum.
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    d3SVG.call(
      d3.zoom<SVGSVGElement, unknown>().on("zoom", function (this: SVGSVGElement, event: unknown) {
        const t = (event as { transform: d3.ZoomTransform }).transform.toString();
        nodeGroup.attr("transform", t);
        edgeGroup.attr("transform", t);
      } as any), // eslint-disable-line @typescript-eslint/no-explicit-any
    );

    const colorScale = d3
      .scaleSequential(d3.interpolateOranges)
      .domain([0, 1]);

    // Nodes
    const node = nodeGroup
      .selectAll<SVGGElement, PProfNode>("g")
      .data(network.nodes, (d: PProfNode) => String(d.func.ID))
      .enter()
      .append("g")
      .attr("class", "pprof-node")
      .attr("transform", (_: PProfNode, i: number) => `translate(0, ${i * 30})`);

    const showTooltip = (event: PointerEvent | MouseEvent, d: PProfNode) => {
      const tip = tooltipRef.current;
      if (!tip) return;
      tip.innerHTML = `
        <div><strong>${d.func.Name}</strong></div>
        <div>Time: ${(d.timePercentage * 100).toFixed(1)}%</div>
        <div>Self: ${(d.selfTimePercentage * 100).toFixed(1)}%</div>
      `;
      tip.style.display = "block";
      tip.style.left = `${event.offsetX + 10}px`;
      tip.style.top = `${event.offsetY + 10}px`;

      // Dim unrelated edges
      edgeGroup
        .selectAll<SVGPathElement, PProfEdge>("path")
        .attr("opacity", (edge: PProfEdge) =>
          edge.caller.index === d.index || edge.callee.index === d.index
            ? "1"
            : "0.3",
        );
    };

    const hideTooltip = () => {
      const tip = tooltipRef.current;
      if (tip) tip.style.display = "none";
      edgeGroup.selectAll("path").attr("opacity", "1");
    };

    // D3 v7 .on() passes (event, datum); v5 types expect (datum, index, ...).
    // Use `as any` to bridge the type mismatch.
    /* eslint-disable @typescript-eslint/no-explicit-any */

    // Total-time rect
    node
      .append("rect")
      .attr("width", 15)
      .attr("height", 15)
      .attr("fill", (d: PProfNode) => colorScale(d.timePercentage) ?? "#fff")
      .attr("stroke", "#000")
      .attr("stroke-width", 1)
      .attr("rx", 5)
      .attr("ry", 5)
      .attr("x", 2)
      .attr("y", 2)
      .on("mouseover", ((e: MouseEvent, d: PProfNode) => {
        showTooltip(e, d);
      }) as any)
      .on("mouseout", hideTooltip as any);

    // Self-time rect
    node
      .append("rect")
      .attr("width", 15)
      .attr("height", 15)
      .attr("fill", (d: PProfNode) => colorScale(d.selfTimePercentage) ?? "#fff")
      .attr("stroke", "#000")
      .attr("stroke-width", 1)
      .attr("rx", 5)
      .attr("ry", 5)
      .attr("x", 20)
      .attr("y", 2)
      .on("mouseover", ((e: MouseEvent, d: PProfNode) => {
        showTooltip(e, d);
      }) as any)
      .on("mouseout", hideTooltip as any);

    /* eslint-enable @typescript-eslint/no-explicit-any */

    // Label
    node
      .append("text")
      .attr("x", 45)
      .attr("y", 15)
      .style("font-size", "12px")
      .text((d: PProfNode) => {
        const parts = d.func.Name.split("/");
        return parts[parts.length - 1];
      });

    // Edges (arcs)
    edgeGroup
      .selectAll<SVGPathElement, PProfEdge>("path")
      .data(Array.from(network.edges.values()))
      .enter()
      .append("path")
      .attr("d", (d: PProfEdge) => {
        const x1 = 0;
        const y1 = d.caller.index * 30 + 7;
        const x2 = -1;
        const y2 = d.callee.index * 30 + 7;
        const r = Math.abs(y2 - y1) / 2;
        return `M ${x1} ${y1} A ${r} ${r} 0 0 0 ${x2} ${y2}`;
      })
      .attr("stroke", "#999")
      .attr("stroke-width", (d: PProfEdge) => Math.max(d.timePercentage * 10, 0.5))
      .attr("fill", "none")
      .attr("marker-end", "url(#arrowhead)");
  }, [network]);

  return (
    <div style={{ position: "relative" }}>
      <div
        ref={tooltipRef}
        style={{
          display: "none",
          position: "absolute",
          background: "rgba(255,255,255,0.95)",
          border: "1px solid #ccc",
          borderRadius: 4,
          padding: "4px 8px",
          fontSize: 12,
          pointerEvents: "none",
          zIndex: 10,
        }}
      />
      <svg
        ref={svgRef}
        width="100%"
        height={Math.max(network.nodes.length * 30 + 20, 200)}
        style={{ overflow: "visible" }}
      />
    </div>
  );
}
