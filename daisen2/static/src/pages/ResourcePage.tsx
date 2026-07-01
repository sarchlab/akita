import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { X } from "lucide-react";
import * as d3 from "d3";
import { useResourceBlocking } from "../hooks/useResourceBlocking";
import { useResourceTasks } from "../hooks/useResourceTasks";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useSegments } from "../hooks/useSegments";
import { useElementSize } from "../hooks/useElementSize";
import TraceChartLayout from "../components/TraceChartLayout";
import TimeTicks from "../components/charts/TimeTicks";
import YAxisOverlay from "../components/charts/YAxisOverlay";
import GapShading from "../components/charts/GapShading";
import TimeZoomControls from "../components/charts/TimeZoomControls";
import SelectedTaskSection from "../components/SelectedTaskSection";
import { Button } from "../components/ui/button";
import { ResourceViewHelp } from "../components/HelpTopics";
import { SectionLabel } from "../components/Legend";
import { milestonesOf } from "../utils/milestoneViz";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../utils/taskColorCoder";
import type { Task } from "../types/task";
import {
  AXIS_TICK_COUNT,
  COLOR_BAR_STROKE,
  COLOR_GRID,
  barOpacity,
  barStrokeOpacity,
  gapSegments,
  safeScale,
} from "../components/charts/chartStyle";

const MIN_RANGE = 1e-12;
const DEBOUNCE_MS = 400;
const AXIS_PAD = 20; // room above/below for the top and bottom time-axis labels
const CURVE_PAD_TOP = 18; // room at the top of the curve band for its label
const GAP = 5;
// Below this many tasks in view, draw the per-task gantt under the curve.
const GANTT_THRESHOLD = 300;
const HW_RESOURCE_KIND = "hardware_resource";
// Warm fill for the blocking-reason (milestone) family.
const FILL = "#f59e0b";
const STROKE = "#ea580c";

interface TimeRange {
  startTime: number;
  endTime: number;
}

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(id);
  }, [delayMs, value]);
  return debounced;
}

function sanitize(start: number, end: number): TimeRange {
  if (Number.isFinite(start) && Number.isFinite(end) && end > start) return { startTime: start, endTime: end };
  return { startTime: 0, endTime: MIN_RANGE };
}

// blockedIntervals returns each [lo, hi] span a task spent blocked on `what` — the
// interval ending at each matching milestone, from the previous milestone (or the
// task's start).
function blockedIntervals(task: Task, what: string): { lo: number; hi: number }[] {
  const ms = milestonesOf(task.steps).slice().sort((a, b) => a.time - b.time);
  const out: { lo: number; hi: number }[] = [];
  for (let i = 0; i < ms.length; i++) {
    if (ms[i].kind === HW_RESOURCE_KIND && ms[i].what === what) {
      out.push({ lo: i > 0 ? ms[i - 1].time : task.start_time, hi: ms[i].time });
    }
  }
  return out;
}

// ResourcePage (/resource?what=<name>) shows one hardware resource like the
// component page shows a location: the occupancy curve of tasks blocked on it
// (always, top) and — when few enough are in view — a per-task gantt below, each
// task drawn in full with its wait for this resource highlighted. Drag pans,
// Cmd/Ctrl+scroll or the buttons zoom, the axis sits top and bottom, and clicking
// a task shows its detail in the side panel.
export default function ResourcePage() {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const what = searchParams.get("what") ?? "";
  const { startTime: simStart, endTime: simEnd } = useSimulationRange();
  const { data: segmentsData } = useSegments();

  const urlStart = Number(searchParams.get("starttime"));
  const urlEnd = Number(searchParams.get("endtime"));
  const urlHasRange =
    searchParams.has("starttime") && Number.isFinite(urlStart) && Number.isFinite(urlEnd) && urlEnd > urlStart;
  const [viewRange, setViewRange] = useState<TimeRange>(
    urlHasRange ? { startTime: urlStart, endTime: urlEnd } : { startTime: simStart, endTime: simEnd },
  );
  const [userZoomed, setUserZoomed] = useState(urlHasRange);
  useEffect(() => {
    if (!userZoomed) setViewRange({ startTime: simStart, endTime: simEnd });
  }, [simStart, simEnd, userZoomed]);

  const dataRange = useDebouncedValue(viewRange, DEBOUNCE_MS);
  const dataPending =
    viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;

  useEffect(() => {
    if (!what) return;
    const params = new URLSearchParams(window.location.search);
    params.set("what", what);
    params.set("starttime", dataRange.startTime.toString());
    params.set("endtime", dataRange.endTime.toString());
    window.history.replaceState(null, "", `/resource?${params.toString()}`);
  }, [dataRange.startTime, dataRange.endTime, what]);

  const { ref, size } = useElementSize<HTMLDivElement>();
  const width = Math.max(size.width, 320);
  const height = Math.max(size.height, 220);
  const innerWidth = Math.max(1, width - 10);
  const numBins = Math.max(60, Math.min(400, Math.round(innerWidth / 4)));

  const { data, loading } = useResourceBlocking(what, dataRange.startTime, dataRange.endTime, numBins);
  const showGantt = !!data && data.total > 0 && data.total <= GANTT_THRESHOLD;
  const { tasks } = useResourceTasks(what, dataRange.startTime, dataRange.endTime, showGantt, GANTT_THRESHOLD);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selectedTask = tasks.find((t) => String(t.id) === selectedId) ?? null;

  const taskColorMap = useMemo(
    () => buildColorMapFromKeys(tasks.map((t) => taskColorKey(t)), "task"),
    [tasks],
  );

  // Pan/zoom state (kept in refs so the wheel listener reads the latest).
  const containerRef = useRef<HTMLDivElement | null>(null);
  const rangeRef = useRef(viewRange);
  rangeRef.current = viewRange;
  const dragRef = useRef<{ x: number; range: TimeRange } | null>(null);
  const didDragRef = useRef(false);

  const applyRange = useCallback((next: TimeRange) => {
    setUserZoomed(true);
    setViewRange(sanitize(next.startTime, next.endTime));
  }, []);
  const zoomBy = useCallback(
    (factor: number) => {
      const r = rangeRef.current;
      const c = (r.startTime + r.endTime) / 2;
      const half = ((r.endTime - r.startTime) / 2) * factor;
      applyRange({ startTime: c - half, endTime: c + half });
    },
    [applyRange],
  );
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const r = rangeRef.current;
      const rect = el.getBoundingClientRect();
      const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left - 5) / Math.max(1, rect.width - 10)));
      const dur = r.endTime - r.startTime;
      const scale = Math.pow(1.0015, event.deltaY);
      const anchor = r.startTime + dur * ratio;
      applyRange({ startTime: anchor - (anchor - r.startTime) * scale, endTime: anchor + (r.endTime - anchor) * scale });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [applyRange]);

  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    dragRef.current = { x: event.clientX, range: rangeRef.current };
    didDragRef.current = false;
  };
  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag) return;
    const dx = event.clientX - drag.x;
    if (Math.abs(dx) > 2) didDragRef.current = true;
    const dur = drag.range.endTime - drag.range.startTime;
    const dt = (dur / Math.max(1, innerWidth)) * dx;
    applyRange({ startTime: drag.range.startTime - dt, endTime: drag.range.endTime - dt });
  };
  const onPointerUp = () => {
    dragRef.current = null;
  };

  const startTime = viewRange.startTime;
  const endTime = viewRange.endTime;
  const xScale = useMemo(
    () => d3.scaleLinear().domain([startTime, endTime]).range([5, width - 5]),
    [startTime, endTime, width],
  );

  // Vertical layout: [top axis] [per-task gantt (optional)] [occupancy curve]
  // [bottom axis] — gantt above the curve, matching the component view's order.
  const gridTop = AXIS_PAD;
  const gridBottom = height - AXIS_PAD;
  const contentH = Math.max(1, gridBottom - gridTop);
  const curveH = showGantt ? Math.min(Math.round(contentH * 0.4), 180) : contentH;
  const curveTop = showGantt ? gridBottom - curveH : gridTop;
  const curveBottom = gridBottom;
  const taskTop = gridTop;
  const taskH = showGantt ? Math.max(0, curveTop - GAP - taskTop) : 0;

  const { areaPath, yScale } = useMemo(() => {
    const bins = data?.bins ?? [];
    const maxV = Math.max(1, d3.max(bins) ?? 1);
    // Leave CURVE_PAD_TOP at the top of the band for the label, matching the
    // component page's task-count area (padTop above the peak).
    const y = d3.scaleLinear().domain([0, maxV]).nice().range([curveBottom, curveTop + CURVE_PAD_TOP]);
    const dStart = data?.start_time ?? startTime;
    const n = data?.num_bins ?? bins.length;
    const binW = n > 0 ? ((data?.end_time ?? endTime) - dStart) / n : 0;
    const pts = bins.map((v, b) => ({ t: dStart + (b + 0.5) * binW, v }));
    const area = d3.area<{ t: number; v: number }>().x((p) => xScale(p.t)).y0(y(0)).y1((p) => y(p.v)).curve(d3.curveMonotoneX);
    return { areaPath: area(pts) ?? "", yScale: y };
  }, [data, xScale, startTime, endTime, curveTop, curveBottom]);

  const gaps = segmentsData?.enabled ? gapSegments(segmentsData.segments, startTime, endTime) : [];
  const hasData = (data?.bins.length ?? 0) > 0;
  const rowH = showGantt && tasks.length > 0 ? taskH / tasks.length : 0;

  const panel = (
    <>
      {/* Header mirrors the component page: title on the left, the Updating…
          pill and a Deselect-task action on the right. */}
      <div className="flex shrink-0 items-start justify-between gap-2 border-b px-4 py-3">
        <div className="min-w-0">
          <SectionLabel>Hardware resource</SectionLabel>
          <div className="mt-0.5 break-all font-mono text-sm font-bold leading-tight">{what || "—"}</div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {(dataPending || loading) && what ? (
            <span className="rounded border border-amber-300 bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700">
              Updating…
            </span>
          ) : null}
          {selectedId ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 gap-1 px-2 text-xs"
              onClick={() => setSelectedId(null)}
              title="Clear the selected task"
            >
              <X className="h-3.5 w-3.5" />
              Deselect task
            </Button>
          ) : null}
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4">
        {/* What the view means moved into the chart-corner info modal
            (ResourceViewHelp); the panel just reflects the selected task. */}
        <SelectedTaskSection task={selectedTask} milestone={null} />
      </div>
    </>
  );

  return (
    <TraceChartLayout panel={panel}>
      <div className="relative min-w-0 flex-1 bg-white">
        <div
          ref={(node) => {
            ref.current = node;
            containerRef.current = node;
          }}
          className="h-full w-full cursor-grab select-none active:cursor-grabbing"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerLeave={onPointerUp}
          onClick={() => {
            if (!didDragRef.current) setSelectedId(null);
          }}
        >
          {!what ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No resource selected.</div>
          ) : loading && !data ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Computing…</div>
          ) : !hasData ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              No blocking recorded for this resource in range.
            </div>
          ) : (
            <svg width={width} height={height} className="block">
              <TimeTicks
                ticks={xScale.ticks(AXIS_TICK_COUNT)}
                xScale={xScale}
                gridTop={gridTop}
                gridBottom={gridBottom}
                topLabelY={12}
                bottomLabelY={height - 6}
                tickMarks
              />
              <line x1={5} x2={width - 5} y1={gridTop} y2={gridTop} stroke={COLOR_GRID} />
              {showGantt ? <line x1={5} x2={width - 5} y1={curveTop} y2={curveTop} stroke={COLOR_GRID} /> : null}
              <line x1={5} x2={width - 5} y1={gridBottom} y2={gridBottom} stroke={COLOR_GRID} />

              {/* Occupancy curve (always) — a filled band only, matching the
                  component page's task-count area (no outline, 0.9 opacity). */}
              <path d={areaPath} fill={FILL} opacity={0.9} />
              <YAxisOverlay yScale={yScale} width={width} />
              <text
                x={8}
                y={curveTop + 13}
                fontSize="11"
                fill="#475569"
                stroke="#ffffff"
                strokeWidth={2.5}
                paintOrder="stroke"
                pointerEvents="none"
              >
                {`Tasks blocked · ${data?.total.toLocaleString() ?? "—"}${
                  data && data.sample > 1 ? ` · ≈1-in-${data.sample} sample` : ""
                }${data && !showGantt && data.total > 0 ? " · zoom in for individual tasks" : ""}`}
              </text>

              {/* Per-task gantt (when few in view): full bar + highlighted wait. */}
              {showGantt &&
                tasks.map((task, i) => {
                  const barY = taskTop + i * rowH + Math.min(1, rowH * 0.15);
                  const barH = Math.max(1.5, rowH - Math.min(2, rowH * 0.3));
                  const bx0 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.start_time)));
                  const bx1 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.end_time)));
                  const selected = selectedId === String(task.id);
                  const intervals = blockedIntervals(task, what);
                  const blocked = intervals.reduce((s, iv) => s + (iv.hi - iv.lo), 0);
                  return (
                    <g
                      key={task.id}
                      className="cursor-pointer"
                      onClick={(event) => {
                        event.stopPropagation();
                        if (!didDragRef.current) setSelectedId(String(task.id));
                      }}
                      onDoubleClick={(event) => {
                        event.stopPropagation();
                        // Open the component view with this task as the current task,
                        // keeping the current time window.
                        const params = new URLSearchParams({
                          name: task.location,
                          taskid: String(task.id),
                          starttime: String(viewRange.startTime),
                          endtime: String(viewRange.endTime),
                        });
                        navigate(`/component?${params.toString()}`);
                      }}
                    >
                      <title>{`${task.kind} ${task.what} @ ${task.location} — blocked ${blocked.toLocaleString()} on ${what}`}</title>
                      {/* The whole task. */}
                      <rect
                        x={bx0}
                        y={barY}
                        width={Math.max(1, bx1 - bx0)}
                        height={barH}
                        fill={lookupColor(taskColorMap, task)}
                        stroke={COLOR_BAR_STROKE}
                        strokeWidth={0.5}
                        strokeOpacity={barStrokeOpacity({ selected, highlighted: false, hasHighlight: false })}
                        opacity={barOpacity({ selected, highlighted: false, hasHighlight: false, hasSelection: selectedId != null })}
                      />
                      {/* The part it spent waiting for this resource. */}
                      {intervals.map((iv, k) => (
                        <rect
                          key={k}
                          x={safeScale(xScale, iv.lo)}
                          y={barY}
                          width={Math.max(1, safeScale(xScale, iv.hi) - safeScale(xScale, iv.lo))}
                          height={barH}
                          fill={STROKE}
                          fillOpacity={0.9}
                        />
                      ))}
                    </g>
                  );
                })}

              <GapShading gaps={gaps} xScale={xScale} height={height} patternId="resource-gap" />
            </svg>
          )}
        </div>

        <TimeZoomControls onZoom={(dir) => zoomBy(dir > 0 ? 1.4 : 0.7)} className="absolute right-2 top-1" />
        {what ? (
          <div className="absolute bottom-2 right-2 z-20" onPointerDown={(e) => e.stopPropagation()}>
            <ResourceViewHelp className="bg-white/85 p-1 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm hover:bg-white" />
          </div>
        ) : null}
      </div>
    </TraceChartLayout>
  );
}
