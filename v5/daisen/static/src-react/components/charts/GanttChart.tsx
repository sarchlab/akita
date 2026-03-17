import { useRef, useEffect, useCallback, useState } from "react";
import * as d3 from "d3";
import type { Task, Segment } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMap, lookupColor, type ColorMap } from "../../utils/taskColorCoder";
import { smartString } from "../../utils/smartValue";

/* ------------------------------------------------------------------ */
/*  Layout constants (mirroring original taskview.ts)                 */
/* ------------------------------------------------------------------ */
const MARGIN = { top: 30, right: 10, bottom: 30, left: 10 };
const PARENT_SECTION_HEIGHT = 20;
const MAIN_SECTION_HEIGHT = 20;
const GROUP_GAP = 12;
const MIN_BAR_HEIGHT = 3;
const MAX_BAR_HEIGHT = 12;

/* ------------------------------------------------------------------ */
/*  Helpers                                                           */
/* ------------------------------------------------------------------ */

/** Wraps a d3 scale call so the return is always a number (defaults to 0). */
function sx(
  scale: d3.ScaleLinear<number, number>,
  value: number,
): number {
  return (scale(value) as number | undefined) ?? 0;
}

/** Compute non-traced (gap) periods between segments in a given view range. */
function nonTracedPeriods(
  segments: Segment[],
  viewStart: number,
  viewEnd: number,
): { start: number; end: number }[] {
  if (segments.length === 0) return [{ start: viewStart, end: viewEnd }];

  const sorted = [...segments].sort((a, b) => a.start_time - b.start_time);
  const gaps: { start: number; end: number }[] = [];

  if (sorted[0].start_time > viewStart) {
    gaps.push({ start: viewStart, end: Math.min(sorted[0].start_time, viewEnd) });
  }
  for (let i = 0; i < sorted.length - 1; i++) {
    const curEnd = sorted[i].end_time;
    const nextStart = sorted[i + 1].start_time;
    if (curEnd < nextStart) {
      const s = Math.max(curEnd, viewStart);
      const e = Math.min(nextStart, viewEnd);
      if (s < e) gaps.push({ start: s, end: e });
    }
  }
  const last = sorted[sorted.length - 1];
  if (last.end_time < viewEnd) {
    gaps.push({ start: Math.max(last.end_time, viewStart), end: viewEnd });
  }
  return gaps;
}

/* ------------------------------------------------------------------ */
/*  Props                                                             */
/* ------------------------------------------------------------------ */
export interface GanttChartProps {
  /** The main task currently selected. */
  mainTask: Task | null;
  /** Parent of the main task (may be null). */
  parentTask: Task | null;
  /** Sub-tasks (children) of the main task. */
  subTasks: Task[];
  /** Traced segments for shading. */
  segments: Segment[];
  /** Whether segment shading is enabled. */
  segmentsEnabled: boolean;
  /** Called when the user clicks a task bar. */
  onSelectTask?: (task: Task) => void;
  /** Called when the user hovers on a task. */
  onHoverTask?: (task: Task | null) => void;
}

/* ------------------------------------------------------------------ */
/*  Component                                                         */
/* ------------------------------------------------------------------ */
export default function GanttChart({
  mainTask,
  parentTask,
  subTasks,
  segments,
  segmentsEnabled,
  onSelectTask,
  onHoverTask,
}: GanttChartProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);

  // Zoom state kept in a ref so the D3 callbacks always see current values.
  const zoomState = useRef({ startTime: 0, endTime: 1 });
  const [, forceRender] = useState(0);

  /* ---------- Derive data ------------------------------------------ */
  const allTasks: Task[] = [];
  if (parentTask) {
    allTasks.push({ ...parentTask, isParentTask: true, isMainTask: false });
  }
  if (mainTask) {
    allTasks.push({ ...mainTask, isMainTask: true, isParentTask: false });
  }
  const subs = subTasks.filter((t) => t && t.id);
  const maxYIndex = subs.length > 0 ? assignYIndices(subs) : 0;
  allTasks.push(...subs);

  // Compute time domain from all tasks
  useEffect(() => {
    if (allTasks.length === 0) return;
    const minT = Math.min(...allTasks.map((t) => t.start_time));
    const maxT = Math.max(...allTasks.map((t) => t.end_time));
    const pad = (maxT - minT) * 0.02 || 1e-12;
    zoomState.current = { startTime: minT - pad, endTime: maxT + pad };
    forceRender((n) => n + 1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mainTask?.id, parentTask?.id, subTasks.length]);

  const colorMap: ColorMap = buildColorMap(allTasks);

  /* ---------- D3 rendering ---------------------------------------- */
  const render = useCallback(() => {
    const svg = svgRef.current;
    const container = containerRef.current;
    if (!svg || !container) return;

    const width = container.clientWidth;
    const headerRegion =
      MARGIN.top +
      GROUP_GAP +
      PARENT_SECTION_HEIGHT +
      GROUP_GAP +
      MAIN_SECTION_HEIGHT +
      GROUP_GAP;
    const subRegionHeight = Math.max(
      60,
      (maxYIndex + 1) * Math.min(MAX_BAR_HEIGHT + 2, 14),
    );
    const height = headerRegion + subRegionHeight + MARGIN.bottom;

    svg.setAttribute("width", String(width));
    svg.setAttribute("height", String(height));

    const { startTime, endTime } = zoomState.current;

    const xScale = d3
      .scaleLinear()
      .domain([startTime, endTime])
      .range([MARGIN.left, width - MARGIN.right]);

    const sel = d3.select(svg);

    // ─── clear everything ────────────────────────────────────────
    sel.selectAll("*").remove();

    // ─── defs (pattern for shading) ──────────────────────────────
    const defs = sel.append("defs");
    const pattern = defs
      .append("pattern")
      .attr("id", "gantt-shade-pattern")
      .attr("patternUnits", "userSpaceOnUse")
      .attr("width", 8)
      .attr("height", 8)
      .attr("patternTransform", "rotate(45)");
    pattern
      .append("rect")
      .attr("width", 8)
      .attr("height", 8)
      .attr("fill", "rgba(128,128,128,0.15)");
    pattern
      .append("line")
      .attr("x1", 0)
      .attr("y1", 0)
      .attr("x2", 0)
      .attr("y2", 8)
      .attr("stroke", "rgba(128,128,128,0.3)")
      .attr("stroke-width", 4);

    // ─── segment shading ─────────────────────────────────────────
    if (segmentsEnabled && segments.length > 0) {
      const gaps = nonTracedPeriods(segments, startTime, endTime);
      const shadingG = sel.append("g").attr("class", "segment-shading");
      gaps.forEach((g) => {
        const gx = sx(xScale, g.start);
        const gw = sx(xScale, g.end) - gx;
        if (gw > 0) {
          shadingG
            .append("rect")
            .attr("x", gx)
            .attr("y", MARGIN.top)
            .attr("width", gw)
            .attr("height", height - MARGIN.top - MARGIN.bottom)
            .attr("fill", "url(#gantt-shade-pattern)")
            .attr("pointer-events", "none");
        }
      });
    }

    // ─── x-axis ──────────────────────────────────────────────────
    const xAxis = d3.axisTop(xScale).ticks(12, "s");
    sel
      .append("g")
      .attr("class", "x-axis")
      .attr("transform", `translate(0,${MARGIN.top})`)
      .call(xAxis);

    // vertical grid lines
    const tickVals = xScale.ticks(12);
    const gridG = sel.append("g").attr("class", "grid");
    tickVals.forEach((tv: number) => {
      gridG
        .append("line")
        .attr("x1", sx(xScale, tv))
        .attr("x2", sx(xScale, tv))
        .attr("y1", MARGIN.top)
        .attr("y2", height - MARGIN.bottom)
        .attr("stroke", "#ccc")
        .attr("stroke-dasharray", "3,3")
        .attr("opacity", 0.5);
    });

    // ─── section labels & dividers ───────────────────────────────
    const labelStyle =
      "font-size:13px; font-weight:600; text-shadow: -1px -1px 0 #fff, 1px -1px 0 #fff, -1px 1px 0 #fff, 1px 1px 0 #fff;";

    let y = MARGIN.top + GROUP_GAP;

    // Parent
    sel
      .append("text")
      .attr("x", 6)
      .attr("y", y + 13)
      .attr("style", labelStyle)
      .text("Parent Task");

    const parentY = y;
    y += PARENT_SECTION_HEIGHT;

    // divider
    y += GROUP_GAP * 0.5;
    sel
      .append("line")
      .attr("x1", 0)
      .attr("x2", width)
      .attr("y1", y)
      .attr("y2", y)
      .attr("stroke", "#000")
      .attr("stroke-dasharray", "4");
    y += GROUP_GAP * 0.5;

    // Main
    sel
      .append("text")
      .attr("x", 6)
      .attr("y", y + 13)
      .attr("style", labelStyle)
      .text("Current Task");

    const mainY = y;
    y += MAIN_SECTION_HEIGHT;

    y += GROUP_GAP * 0.5;
    sel
      .append("line")
      .attr("x1", 0)
      .attr("x2", width)
      .attr("y1", y)
      .attr("y2", y)
      .attr("stroke", "#000")
      .attr("stroke-dasharray", "4");
    y += GROUP_GAP * 0.5;

    sel
      .append("text")
      .attr("x", 6)
      .attr("y", y + 13)
      .attr("style", labelStyle + "pointer-events:none;")
      .text("Subtasks");

    const subBaseY = y;

    // ─── bar height for subtasks ─────────────────────────────────
    const subSpace = subRegionHeight;
    let barH = maxYIndex > 0 ? subSpace / (maxYIndex + 1) : MAX_BAR_HEIGHT;
    barH = Math.max(MIN_BAR_HEIGHT, Math.min(MAX_BAR_HEIGHT, barH));

    // ─── task bars ───────────────────────────────────────────────
    const barsG = sel.append("g").attr("class", "task-bars");

    function taskBarY(t: Task): number {
      if (t.isParentTask) return parentY;
      if (t.isMainTask) return mainY;
      return subBaseY + (t.yIndex ?? 0) * (barH + 1);
    }

    function taskBarH(t: Task): number {
      if (t.isParentTask) return PARENT_SECTION_HEIGHT;
      if (t.isMainTask) return MAIN_SECTION_HEIGHT;
      return barH * 0.85;
    }

    allTasks.forEach((t) => {
      const tx = sx(xScale, t.start_time);
      const tw = sx(xScale, t.end_time) - tx;
      if (tw < 0.2) return; // skip invisible bars

      const rect = barsG
        .append("rect")
        .attr("x", tx)
        .attr("y", taskBarY(t))
        .attr("width", Math.max(1, tw))
        .attr("height", taskBarH(t))
        .attr("fill", lookupColor(colorMap, t))
        .attr("stroke", "#000")
        .attr("stroke-opacity", 0.2)
        .attr("rx", 2)
        .style("cursor", "pointer");

      rect.on("click", () => onSelectTask?.(t));
      rect.on("mouseenter", () => {
        rect.attr("stroke-opacity", 0.8).attr("stroke-width", 1.5);
        onHoverTask?.(t);
      });
      rect.on("mouseleave", () => {
        rect.attr("stroke-opacity", 0.2).attr("stroke-width", 1);
        onHoverTask?.(null);
      });

      // tooltip title
      rect.append("title").text(
        `${t.kind} - ${t.what}\n${t.location}\n${smartString(t.start_time)} → ${smartString(t.end_time)}  (${smartString(t.end_time - t.start_time)})`,
      );
    });

    // ─── milestones (red dots) for main task ─────────────────────
    if (mainTask && mainTask.steps && mainTask.steps.length > 0) {
      const dotsG = sel.append("g").attr("class", "milestones");
      const cy = mainY + MAIN_SECTION_HEIGHT / 2;
      mainTask.steps.forEach((s) => {
        const cx = sx(xScale, s.time);
        dotsG
          .append("circle")
          .attr("cx", cx)
          .attr("cy", cy)
          .attr("r", 3)
          .attr("fill", "red")
          .attr("stroke", "#fff")
          .attr("stroke-width", 1)
          .append("title")
          .text(`${s.kind}: ${s.what} @ ${smartString(s.time)}`);
      });
    }

    // ─── bottom x-axis ──────────────────────────────────────────
    const xAxisBottom = d3.axisBottom(xScale).ticks(12, "s");
    sel
      .append("g")
      .attr("class", "x-axis-bottom")
      .attr("transform", `translate(0,${height - MARGIN.bottom})`)
      .call(xAxisBottom);
  }, [
    allTasks,
    mainTask,
    maxYIndex,
    colorMap,
    segments,
    segmentsEnabled,
    onSelectTask,
    onHoverTask,
  ]);

  /* ---------- Zoom / pan via mouse events (like MouseEventHandler) */
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    let isDragging = false;
    let dragStartX = 0;
    let dragStartStart = 0;
    let dragStartEnd = 0;
    let scrollTimer: ReturnType<typeof setTimeout> | null = null;

    const scheduleRerender = () => {
      if (scrollTimer) clearTimeout(scrollTimer);
      scrollTimer = setTimeout(() => forceRender((n) => n + 1), 16);
    };

    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const { startTime, endTime } = zoomState.current;
      const w = container.clientWidth;
      const duration = endTime - startTime;
      const tpp = duration / (w - MARGIN.left - MARGIN.right);

      if (e.deltaY !== 0) {
        // Zoom around cursor
        const pxLeft = e.offsetX - MARGIN.left;
        const pxRight = w - MARGIN.right - e.offsetX;
        const mouseTime = startTime + pxLeft * tpp;
        let newTpp = tpp;
        const steps = Math.abs(e.deltaY);
        for (let i = 0; i < steps; i++) {
          newTpp *= e.deltaY > 0 ? 1.001 : 1 / 1.001;
        }
        zoomState.current = {
          startTime: mouseTime - newTpp * pxLeft,
          endTime: mouseTime + newTpp * pxRight,
        };
        scheduleRerender();
      }

      if (e.deltaX !== 0) {
        const shift = duration * e.deltaX * 0.001;
        zoomState.current = {
          startTime: startTime + shift,
          endTime: endTime + shift,
        };
        scheduleRerender();
      }
    };

    const onMouseDown = (e: MouseEvent) => {
      isDragging = true;
      dragStartX = e.offsetX;
      const { startTime, endTime } = zoomState.current;
      dragStartStart = startTime;
      dragStartEnd = endTime;
    };

    const onMouseMove = (e: MouseEvent) => {
      if (!isDragging) return;
      const w = container.clientWidth;
      const duration = dragStartEnd - dragStartStart;
      const tpp = duration / (w - MARGIN.left - MARGIN.right);
      const delta = e.offsetX - dragStartX;
      const timeDelta = tpp * delta;
      zoomState.current = {
        startTime: dragStartStart - timeDelta,
        endTime: dragStartEnd - timeDelta,
      };
      scheduleRerender();
    };

    const onMouseUp = () => {
      isDragging = false;
    };

    container.addEventListener("wheel", onWheel, { passive: false });
    container.addEventListener("mousedown", onMouseDown);
    container.addEventListener("mousemove", onMouseMove);
    container.addEventListener("mouseup", onMouseUp);
    container.addEventListener("mouseleave", onMouseUp);

    return () => {
      container.removeEventListener("wheel", onWheel);
      container.removeEventListener("mousedown", onMouseDown);
      container.removeEventListener("mousemove", onMouseMove);
      container.removeEventListener("mouseup", onMouseUp);
      container.removeEventListener("mouseleave", onMouseUp);
      if (scrollTimer) clearTimeout(scrollTimer);
    };
  }, []);

  /* ---------- Re-render when data or zoom changes ------------- */
  useEffect(() => {
    render();
  }, [render]);

  /* ---------- Resize observer --------------------------------- */
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver(() => {
      render();
    });
    ro.observe(container);
    return () => ro.disconnect();
  }, [render]);

  /* ---------- Color legend ------------------------------------ */
  const legendEntries = Object.entries(colorMap).sort((a, b) =>
    a[0].localeCompare(b[0]),
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div
        ref={containerRef}
        style={{
          flex: 1,
          overflow: "hidden",
          cursor: "grab",
          minHeight: 200,
          position: "relative",
        }}
      >
        <svg ref={svgRef} style={{ display: "block" }} />
      </div>

      {/* Color legend */}
      {legendEntries.length > 0 && (
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: 12,
            padding: "6px 10px",
            borderTop: "1px solid #dee2e6",
            fontSize: 12,
          }}
        >
          {legendEntries.map(([key, color]) => (
            <span key={key} style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <span
                style={{
                  display: "inline-block",
                  width: 12,
                  height: 12,
                  background: color,
                  borderRadius: 2,
                  border: "1px solid #ccc",
                }}
              />
              {key}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
