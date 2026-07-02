import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  WheelEvent as ReactWheelEvent,
} from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Minus, Plus, X } from "lucide-react";
import * as d3 from "d3";
import { useResourceBlocking } from "../hooks/useResourceBlocking";
import { useResourceTasks } from "../hooks/useResourceTasks";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useSegments } from "../hooks/useSegments";
import { useElementSize } from "../hooks/useElementSize";
import { useDebouncedValue } from "../hooks/useDebouncedValue";
import { useAutoColorMode } from "../hooks/useAutoColorMode";
import { useColorMapFromKeys } from "../hooks/useTaskColorMap";
import TraceChartLayout from "../components/TraceChartLayout";
import TimeTicks from "../components/charts/TimeTicks";
import YAxisOverlay from "../components/charts/YAxisOverlay";
import GapShading from "../components/charts/GapShading";
import TimeZoomControls, { ZOOM_BTN_CLASS } from "../components/charts/TimeZoomControls";
import SelectedTaskSection from "../components/SelectedTaskSection";
import { Button } from "../components/ui/button";
import { ResourceViewHelp } from "../components/HelpTopics";
import Legend from "../components/Legend";
import { SectionLabel } from "../components/Legend";
import { mergeConsecutiveMilestones, milestonesOf, wavyPath } from "../utils/milestoneViz";
import { lookupColor, taskColorKey } from "../utils/taskColorCoder";
import type { ColorMode } from "../utils/taskColorCoder";
import { assignYIndices } from "../utils/taskYIndexAssigner";
import type { Task } from "../types/task";
import {
  AXIS_TICK_COUNT,
  COLOR_HALO,
  COLOR_BAR_STROKE,
  COLOR_GRID,
  MILESTONE_DOT_R,
  MILESTONE_WAVE_WIDTH,
  OPACITY_DIM_MILESTONE,
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
const ROW_HEIGHT = 16;
const MIN_ROW_HEIGHT = 4;
const MAX_ROW_HEIGHT = 80;
const HW_RESOURCE_KIND = "hardware_resource";
// Warm fill for the blocking-reason (milestone) family.
const FILL = "#f59e0b";
const STROKE = "#ea580c";

interface TimeRange {
  startTime: number;
  endTime: number;
}

interface PackedTasks {
  tasks: Task[];
  rows: number;
}

function sanitize(start: number, end: number): TimeRange {
  if (Number.isFinite(start) && Number.isFinite(end) && end > start) return { startTime: start, endTime: end };
  return { startTime: 0, endTime: MIN_RANGE };
}

function sameTime(a: number, b: number): boolean {
  return Math.abs(a - b) <= Math.max(1e-9, Math.abs(b) * 1e-12);
}

// blockedIntervals returns each [lo, hi] span a task spent blocked on `what` — the
// interval ending at each matching milestone, from the previous milestone (or the
// task's start).
function blockedIntervals(task: Task, what: string): { lo: number; hi: number }[] {
  const ms = mergeConsecutiveMilestones(milestonesOf(task.steps).slice().sort((a, b) => a.time - b.time));
  const out: { lo: number; hi: number }[] = [];
  for (let i = 0; i < ms.length; i++) {
    if (ms[i].kind === HW_RESOURCE_KIND && ms[i].what === what) {
      out.push({ lo: i > 0 ? ms[i - 1].time : task.start_time, hi: ms[i].time });
    }
  }
  return out;
}

function isBlockedOnResourceAt(task: Task, what: string, time: number): boolean {
  return blockedIntervals(task, what).some((interval) => interval.lo <= time && time <= interval.hi);
}

function packTasksUp(tasks: Task[]): PackedTasks {
  const packed = tasks.filter((task) => task.end_time > task.start_time).map((task) => ({ ...task }));
  if (packed.length === 0) return { tasks: [], rows: 0 };
  const maxRow = assignYIndices(packed);
  return { tasks: packed, rows: maxRow + 1 };
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

  const { ref: sizeRef, size } = useElementSize<HTMLDivElement>();
  const width = Math.max(size.width, 320);
  const height = Math.max(size.height, 220);
  const innerWidth = Math.max(1, width - 10);
  const numBins = Math.max(60, Math.min(400, Math.round(innerWidth / 4)));

  const { data, loading } = useResourceBlocking(what, dataRange.startTime, dataRange.endTime, numBins);
  const dataMatchesRange =
    !!data &&
    sameTime(data.start_time, dataRange.startTime) &&
    sameTime(data.end_time, dataRange.endTime) &&
    data.num_bins === numBins;
  const [showGantt, setShowGantt] = useState(false);
  useEffect(() => {
    setShowGantt(false);
  }, [what]);
  useEffect(() => {
    if (dataMatchesRange) setShowGantt(!!data && data.total > 0 && data.total <= GANTT_THRESHOLD);
  }, [dataMatchesRange, data?.total]);
  const taskFetchEnabled = dataMatchesRange && showGantt;
  const { tasks: fetchedTasks, loading: tasksLoading } = useResourceTasks(
    what,
    dataRange.startTime,
    dataRange.endTime,
    taskFetchEnabled,
    GANTT_THRESHOLD,
  );
  const [tasks, setTasks] = useState<Task[]>([]);
  useEffect(() => {
    setTasks([]);
  }, [what]);
  useEffect(() => {
    if (taskFetchEnabled && !tasksLoading) setTasks(fetchedTasks);
  }, [fetchedTasks, taskFetchEnabled, tasksLoading]);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selectedTask = tasks.find((t) => String(t.id) === selectedId) ?? null;
  const [colorMode, setColorMode] = useState<ColorMode>("kind-what");
  const [highlightedKey, setHighlightedKey] = useState<string | null>(null);
  const [hoveredResourceTime, setHoveredResourceTime] = useState<number | null>(null);
  const [highlightedResourceReason, setHighlightedResourceReason] = useState<string | null>(null);
  const [rowHeight, setRowHeight] = useState(ROW_HEIGHT);
  // Tasks located at the resource are the work that consumes it (e.g. inst-VALU
  // tasks at GPU[..].VALU). Other tasks carrying the resource milestone are the
  // higher-level tasks blocked by it (e.g. wavefront tasks).
  const usageTasks = useMemo(() => tasks.filter((task) => task.location === what), [tasks, what]);
  const blockedTasks = useMemo(
    () => tasks.filter((task) => task.location !== what && blockedIntervals(task, what).length > 0),
    [tasks, what],
  );
  const usageLayout = useMemo(() => packTasksUp(usageTasks), [usageTasks]);
  const blockedLayout = useMemo(() => packTasksUp(blockedTasks), [blockedTasks]);
  const taskKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const task of tasks) keys.add(taskColorKey(task, colorMode));
    return Array.from(keys).sort();
  }, [tasks, colorMode]);
  const handleColorMode = useAutoColorMode(colorMode, setColorMode, taskKeys.length, 10);
  const taskColorMap = useColorMapFromKeys(taskKeys, "task");
  const resourceReason = what ? taskColorKey({ kind: HW_RESOURCE_KIND, what }, "kind-what") : "";
  const resourceReasonColorMap = useMemo(
    () => (resourceReason ? { [resourceReason]: STROKE } : {}),
    [resourceReason],
  );
  const reasonHighlight = highlightedResourceReason ?? (hoveredResourceTime != null ? resourceReason : null);
  const highlightedBlockedTaskIds = useMemo(() => {
    if (hoveredResourceTime == null || !showGantt) return null;
    const ids = new Set<string>();
    for (const task of blockedTasks) {
      if (task.start_time > hoveredResourceTime || task.end_time < hoveredResourceTime) continue;
      if (isBlockedOnResourceAt(task, what, hoveredResourceTime)) ids.add(String(task.id));
    }
    return ids;
  }, [blockedTasks, hoveredResourceTime, showGantt, what]);

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
  const crosshairRef = useRef<HTMLDivElement | null>(null);
  const moveCrosshair = (event: ReactMouseEvent<HTMLDivElement>) => {
    const line = crosshairRef.current;
    if (!line) return;
    const x = event.clientX - event.currentTarget.getBoundingClientRect().left;
    line.style.transform = `translateX(${Math.round(x)}px)`;
    line.style.opacity = "1";
  };
  const hideCrosshair = () => {
    if (crosshairRef.current) crosshairRef.current.style.opacity = "0";
  };

  const startTime = viewRange.startTime;
  const endTime = viewRange.endTime;
  const xScale = useMemo(
    () => d3.scaleLinear().domain([startTime, endTime]).range([5, width - 5]),
    [startTime, endTime, width],
  );

  // Vertical layout when individual tasks are visible:
  // [resource usage tasks] [blocked tasks + wait waves] [blocked-task occupancy curve].
  // Usage and blocking are separate chart regions, divided with the same subtle
  // border used by the component page instead of painting both in one gantt.
  const curveRegionHeight = showGantt ? Math.min(Math.round(height * 0.28), 180) : height;
  const ganttRegionHeight = showGantt ? Math.max(1, height - curveRegionHeight) : 0;
  const taskRegionHeight = showGantt ? Math.max(1, Math.round(ganttRegionHeight * 0.5)) : 0;
  const blockingRegionHeight = showGantt ? Math.max(1, ganttRegionHeight - taskRegionHeight) : 0;
  const curveGridTop = showGantt ? 0 : AXIS_PAD;
  const curveGridBottom = Math.max(curveGridTop + 1, curveRegionHeight - AXIS_PAD);
  const taskGridTop = AXIS_PAD;
  const taskTop = taskGridTop;
  const blockingGridTop = 0;
  const blockingLabelTop = 16;
  const blockingTaskTop = 20;
  const taskContentHeight = showGantt
    ? Math.max(taskRegionHeight, taskTop + GAP + usageLayout.rows * rowHeight)
    : 0;
  const blockingContentHeight = showGantt
    ? Math.max(blockingRegionHeight, blockingTaskTop + GAP + blockedLayout.rows * rowHeight)
    : 0;
  const taskGridBottom = Math.max(taskGridTop + 1, taskContentHeight);
  const blockingGridBottom = Math.max(1, blockingContentHeight);
  const taskH = showGantt ? Math.max(0, taskContentHeight - GAP - taskTop) : 0;
  const blockingTaskH = showGantt ? Math.max(0, blockingContentHeight - GAP - blockingTaskTop) : 0;

  const { areaPath, yScale } = useMemo(() => {
    const bins = data?.bins ?? [];
    const maxV = Math.max(1, d3.max(bins) ?? 1);
    // Leave CURVE_PAD_TOP at the top of the band for the label, matching the
    // component page's task-count area (padTop above the peak).
    const y = d3.scaleLinear().domain([0, maxV]).nice().range([curveGridBottom, curveGridTop + CURVE_PAD_TOP]);
    const dStart = data?.start_time ?? startTime;
    const n = data?.num_bins ?? bins.length;
    const binW = n > 0 ? ((data?.end_time ?? endTime) - dStart) / n : 0;
    const pts = bins.map((v, b) => ({ t: dStart + (b + 0.5) * binW, v }));
    const area = d3
      .area<{ t: number; v: number }>()
      .x((p) => safeScale(xScale, p.t))
      .y0(safeScale(y, 0))
      .y1((p) => safeScale(y, p.v))
      .curve(d3.curveMonotoneX);
    return { areaPath: area(pts) ?? "", yScale: y };
  }, [data, xScale, startTime, endTime, curveGridTop, curveGridBottom]);

  const gaps = segmentsData?.enabled ? gapSegments(segmentsData.segments, startTime, endTime) : [];
  const hasData = (data?.bins.length ?? 0) > 0;
  const rowH = showGantt && usageLayout.rows > 0 ? taskH / usageLayout.rows : 0;
  const blockingRowH = showGantt && blockedLayout.rows > 0 ? blockingTaskH / blockedLayout.rows : 0;
  const chartUpdating =
    what && ((!hasData && loading) || (showGantt && taskFetchEnabled && tasks.length === 0));

  const zoomRowsBy = useCallback((dir: number) => {
    setRowHeight((h) => Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, h + dir * 4)));
  }, []);
  const zoomRowsAll = useCallback(() => {
    const fits = [];
    if (usageLayout.rows > 0) fits.push((taskRegionHeight - taskTop - GAP) / usageLayout.rows);
    if (blockedLayout.rows > 0) fits.push((blockingRegionHeight - blockingTaskTop - GAP) / blockedLayout.rows);
    if (fits.length === 0) return;
    setRowHeight(Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, Math.floor(Math.min(...fits)))));
  }, [blockedLayout.rows, blockingRegionHeight, blockingTaskTop, taskRegionHeight, taskTop, usageLayout.rows]);
  const onRowsWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (!event.altKey) return;
    event.preventDefault();
    event.stopPropagation();
    setRowHeight((h) => Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, h - event.deltaY * 0.04)));
  };

  const rowControls = showGantt ? (
    <div
      className="absolute right-2 top-9 z-20 flex items-center gap-0.5 rounded border bg-white/90 px-1 py-0.5 shadow-sm"
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className="select-none px-0.5 text-[10px] font-medium text-muted-foreground">rows</span>
      <button type="button" className={ZOOM_BTN_CLASS} title="Shorter rows (Alt+scroll)" onClick={() => zoomRowsBy(-1)}>
        <Minus className="h-4 w-4" />
      </button>
      <button type="button" className={ZOOM_BTN_CLASS} title="Taller rows (Alt+scroll)" onClick={() => zoomRowsBy(1)}>
        <Plus className="h-4 w-4" />
      </button>
      <button
        type="button"
        className={`${ZOOM_BTN_CLASS} px-1 text-[10px] font-medium`}
        title="Fit all rows"
        onClick={zoomRowsAll}
      >
        all
      </button>
    </div>
  ) : null;

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
          {(dataPending || loading || tasksLoading || (what && data && !dataMatchesRange)) && what ? (
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
        <div className="-mx-4 border-t" />
        <Legend
          taskKeys={taskKeys}
          colorMap={taskColorMap}
          blockingReasons={resourceReason ? [resourceReason] : []}
          milestoneColorMap={resourceReasonColorMap}
          colorMode={colorMode}
          onColorMode={handleColorMode}
          highlightedKey={highlightedKey}
          onHighlight={setHighlightedKey}
          highlightedReason={reasonHighlight}
          onHighlightReason={setHighlightedResourceReason}
          resourceRange={viewRange}
        />
      </div>
    </>
  );

  return (
    <TraceChartLayout panel={panel}>
      <div
        className="daisen1-component-left min-w-0 flex-1 bg-white"
        onMouseMove={moveCrosshair}
        onMouseLeave={() => {
          hideCrosshair();
          setHoveredResourceTime(null);
        }}
      >
        <div
          ref={(node) => {
            sizeRef(node);
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
          ) : chartUpdating ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Updating…</div>
          ) : !hasData ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              No blocking recorded for this resource in range.
            </div>
          ) : (
            <>
              {showGantt ? (
                <div className="daisen1-component-view relative" style={{ height: taskRegionHeight }}>
                  <div className="h-full w-full overflow-y-auto overflow-x-hidden" onWheel={onRowsWheel}>
                    <svg width={width} height={taskContentHeight} className="block">
                      <TimeTicks
                        ticks={xScale.ticks(AXIS_TICK_COUNT)}
                        xScale={xScale}
                        gridTop={taskGridTop}
                        gridBottom={taskGridBottom}
                        topLabelY={12}
                        tickMarks
                      />
                      <line x1={5} x2={width - 5} y1={taskGridTop} y2={taskGridTop} stroke={COLOR_GRID} />
                      <GapShading gaps={gaps} xScale={xScale} height={taskContentHeight} patternId="resource-task-gap" />

                      {usageLayout.tasks.map((task) => {
                      const barY = taskTop + (task.yIndex ?? 0) * rowH + Math.min(1, rowH * 0.15);
                      const barH = Math.max(1.5, rowH - Math.min(2, rowH * 0.3));
                      const bx0 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.start_time)));
                      const bx1 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.end_time)));
                      const selected = selectedId === String(task.id);
                      const key = taskColorKey(task, colorMode);
                      const hasHighlight = highlightedKey !== null;
                      const highlighted = highlightedKey !== null ? highlightedKey === key : true;
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
                          <title>{`${task.kind} ${task.what} @ ${task.location}`}</title>
                          {/* The whole task. */}
                          <rect
                            x={bx0}
                            y={barY}
                            width={Math.max(1, bx1 - bx0)}
                            height={barH}
                            fill={lookupColor(taskColorMap, task, colorMode)}
                            stroke={COLOR_BAR_STROKE}
                            strokeWidth={0.5}
                            strokeOpacity={barStrokeOpacity({ selected, highlighted, hasHighlight })}
                            opacity={barOpacity({ selected, highlighted, hasHighlight, hasSelection: selectedId != null })}
                          />
                        </g>
                      );
                      })}

                      <text
                        x={8}
                        y={taskGridTop + 13}
                        fontSize="11"
                        fill="#475569"
                        stroke="#ffffff"
                        strokeWidth={2.5}
                        paintOrder="stroke"
                        pointerEvents="none"
                      >
                        {`Task usage · ${usageTasks.length.toLocaleString()} tasks`}
                      </text>
                    </svg>
                  </div>
                </div>
              ) : null}
              {showGantt ? (
                <div className="daisen1-resource-blocking-view relative border-t border-slate-200" style={{ height: blockingRegionHeight }}>
                  <div className="h-full w-full overflow-y-auto overflow-x-hidden" onWheel={onRowsWheel}>
                    <svg width={width} height={blockingContentHeight} className="block">
                      <TimeTicks
                        ticks={xScale.ticks(AXIS_TICK_COUNT)}
                        xScale={xScale}
                        gridTop={blockingGridTop}
                        gridBottom={blockingGridBottom}
                      />
                      <GapShading gaps={gaps} xScale={xScale} height={blockingContentHeight} patternId="resource-blocking-gap" />
                      {blockedLayout.tasks.map((task) => {
                      const centerY = blockingTaskTop + (task.yIndex ?? 0) * blockingRowH + blockingRowH / 2;
                      const barH = Math.max(1.5, blockingRowH - Math.min(2, blockingRowH * 0.3));
                      const barY = centerY - barH / 2;
                      const bx0 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.start_time)));
                      const bx1 = Math.max(5, Math.min(width - 5, safeScale(xScale, task.end_time)));
                      const selected = selectedId === String(task.id);
                      const key = taskColorKey(task, colorMode);
                      const hasHighlight = highlightedBlockedTaskIds !== null || highlightedKey !== null;
                      const highlighted =
                        highlightedBlockedTaskIds !== null
                          ? highlightedBlockedTaskIds.has(String(task.id))
                          : highlightedKey !== null
                            ? highlightedKey === key
                            : true;
                      const intervals = blockedIntervals(task, what);
                      const opacity = hasHighlight && !highlighted ? OPACITY_DIM_MILESTONE : selectedId != null && !selected ? 0.35 : 1;
                      const amplitude = Math.max(1, Math.min(3, blockingRowH * 0.22));
                      const dotR = Math.max(1.2, Math.min(MILESTONE_DOT_R, blockingRowH * 0.35));
                      const blocked = intervals.reduce((sum, iv) => sum + (iv.hi - iv.lo), 0);
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
                          <rect
                            x={bx0}
                            y={barY}
                            width={Math.max(1, bx1 - bx0)}
                            height={barH}
                            fill={lookupColor(taskColorMap, task, colorMode)}
                            stroke={COLOR_BAR_STROKE}
                            strokeWidth={0.5}
                            strokeOpacity={barStrokeOpacity({ selected, highlighted, hasHighlight })}
                            opacity={barOpacity({ selected, highlighted, hasHighlight, hasSelection: selectedId != null })}
                          />
                          {intervals.map((iv, k) => {
                            const lo = Math.max(iv.lo, startTime);
                            const hi = Math.min(iv.hi, endTime);
                            if (hi <= lo) return null;
                            const x0 = safeScale(xScale, lo);
                            const x1 = safeScale(xScale, hi);
                            if (x1 - x0 < 1) return null;
                            const showRelease = iv.hi >= startTime && iv.hi <= endTime;
                            return (
                              <g key={`${task.id}-${k}`}>
                                <rect
                                  x={x0}
                                  y={centerY - 8}
                                  width={x1 - x0}
                                  height={16}
                                  fill="transparent"
                                  pointerEvents="all"
                                />
                                <path
                                  d={wavyPath(x0, x1, centerY, amplitude, 3)}
                                  fill="none"
                                  stroke={STROKE}
                                  strokeWidth={MILESTONE_WAVE_WIDTH}
                                  strokeLinecap="round"
                                  opacity={opacity}
                                  pointerEvents="none"
                                />
                                {showRelease ? (
                                  <circle
                                    cx={x1}
                                    cy={centerY}
                                    r={dotR}
                                    fill={STROKE}
                                    stroke={COLOR_HALO}
                                    strokeWidth={0.75}
                                    opacity={opacity}
                                    pointerEvents="none"
                                  />
                                ) : null}
                              </g>
                            );
                          })}
                        </g>
                      );
                      })}

                      <text
                        x={8}
                        y={blockingLabelTop}
                        fontSize="11"
                        fill="#475569"
                        stroke="#ffffff"
                        strokeWidth={2.5}
                        paintOrder="stroke"
                        pointerEvents="none"
                      >
                        {`Resource waits · ${blockedTasks.length.toLocaleString()} tasks`}
                      </text>
                    </svg>
                  </div>
                </div>
              ) : null}
              <div
                className={`daisen1-metric-view relative${showGantt ? " border-t border-slate-200" : ""}`}
                style={{ height: curveRegionHeight }}
              >
                <svg
                  width={width}
                  height={curveRegionHeight}
                  className="block"
                  onMouseMove={(event) => {
                    const rect = event.currentTarget.getBoundingClientRect();
                    setHoveredResourceTime(xScale.invert(event.clientX - rect.left));
                  }}
                  onMouseLeave={() => setHoveredResourceTime(null)}
                >
                  <TimeTicks
                    ticks={xScale.ticks(AXIS_TICK_COUNT)}
                    xScale={xScale}
                    gridTop={curveGridTop}
                    gridBottom={curveGridBottom}
                    topLabelY={showGantt ? undefined : 12}
                    bottomLabelY={curveRegionHeight - 6}
                    tickMarks
                  />
                  {!showGantt ? <line x1={5} x2={width - 5} y1={curveGridTop} y2={curveGridTop} stroke={COLOR_GRID} /> : null}
                  <line x1={5} x2={width - 5} y1={curveGridBottom} y2={curveGridBottom} stroke={COLOR_GRID} />

                  {/* Occupancy curve (always) — a filled band only, matching the
                      component page's task-count area (no outline, 0.9 opacity). */}
                  <path d={areaPath} fill={FILL} opacity={0.9} />
                  <YAxisOverlay yScale={yScale} width={width} />
                  <text
                    x={8}
                    y={curveGridTop + 13}
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

                  <GapShading gaps={gaps} xScale={xScale} height={curveGridBottom} patternId="resource-curve-gap" />
                </svg>
              </div>
            </>
          )}
        </div>

        <TimeZoomControls onZoom={(dir) => zoomBy(dir > 0 ? 1.4 : 0.7)} className="absolute right-2 top-1" />
        {rowControls}
        {what ? (
          <div className="absolute bottom-2 right-2 z-20" onPointerDown={(e) => e.stopPropagation()}>
            <ResourceViewHelp className="bg-white/85 p-1 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm hover:bg-white" />
          </div>
        ) : null}
        <div
          ref={crosshairRef}
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 left-0 z-10 w-px bg-slate-700/70 opacity-0"
          style={{ transform: "translateX(-1px)", willChange: "transform" }}
        />
      </div>
    </TraceChartLayout>
  );
}
