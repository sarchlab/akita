import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  ReactNode,
  MouseEvent as ReactMouseEvent,
  PointerEvent as ReactPointerEvent,
  WheelEvent as ReactWheelEvent,
} from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ChevronDown, ChevronUp, Minus, Plus, X } from "lucide-react";
import * as d3 from "d3";
import { useResourceBlocking } from "../hooks/useResourceBlocking";
import { useResourceTasks } from "../hooks/useResourceTasks";
import { useComponentTimeline } from "../hooks/useComponentTimeline";
import { useTraceData } from "../hooks/useTraceData";
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
import LoadingCurve from "../components/charts/LoadingCurve";
import TimeZoomControls, { ZOOM_BTN_CLASS } from "../components/charts/TimeZoomControls";
import SelectedTaskSection from "../components/SelectedTaskSection";
import { Button } from "../components/ui/button";
import {
  ResourceUsageCountHelp,
  ResourceUsageTasksHelp,
  ResourceWaitCountHelp,
  ResourceWaitTasksHelp,
} from "../components/HelpTopics";
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
// Below these task counts, draw per-task gantts under the curves. Wait rows are
// the resource bottleneck evidence the user asked to quadruple; usage rows can
// tolerate a slightly higher cap because they are simple bars without waves.
const WAIT_GANTT_THRESHOLD = 1200;
const USAGE_GANTT_THRESHOLD = 2000;
const ROW_HEIGHT = 16;
const MIN_ROW_HEIGHT = 4;
const MAX_ROW_HEIGHT = 80;
const HW_RESOURCE_KIND = "hardware_resource";
const CHART_HELP_CORNER = "absolute bottom-2 right-2 z-20";
const CHART_HELP_BUTTON = "bg-white/85 p-1 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm hover:bg-white";
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

interface ScrollHint {
  canUp: boolean;
  hiddenAbove: number;
  canDown: boolean;
  hiddenBelow: number;
}

interface ResourceGanttScrollFrameProps {
  height: number;
  contentHeight: number;
  rowHeight: number;
  rowCount: number;
  topPad: number;
  controlsTop?: number;
  onRowHeightChange: (value: number | ((prev: number) => number)) => void;
  children: ReactNode;
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

function shortResourceLabel(what: string): string {
  const parts = what.split(".");
  return parts[parts.length - 1] || what || "resource";
}

function componentBinsTotal(bins: number[][]): number[] {
  return bins.map((row) => row.reduce((sum, value) => sum + value, 0));
}

function uniqueTasks(...groups: Task[][]): Task[] {
  const seen = new Set<string>();
  const out: Task[] = [];
  for (const group of groups) {
    for (const task of group) {
      const id = String(task.id);
      if (seen.has(id)) continue;
      seen.add(id);
      out.push(task);
    }
  }
  return out;
}

function ResourceGanttScrollFrame({
  height,
  contentHeight,
  rowHeight,
  rowCount,
  topPad,
  controlsTop = 4,
  onRowHeightChange,
  children,
}: ResourceGanttScrollFrameProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scrollHint, setScrollHint] = useState<ScrollHint>({
    canUp: false,
    hiddenAbove: 0,
    canDown: false,
    hiddenBelow: 0,
  });

  const updateScrollHint = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const above = el.scrollTop;
    const below = el.scrollHeight - el.clientHeight - el.scrollTop;
    const threshold = Math.max(2, rowHeight * 0.5);
    const rows = (px: number) => Math.max(0, Math.ceil(px / Math.max(1, rowHeight)));
    setScrollHint({
      canUp: above > threshold,
      hiddenAbove: rows(above),
      canDown: below > threshold,
      hiddenBelow: rows(below),
    });
  }, [rowHeight]);

  useEffect(() => {
    updateScrollHint();
  }, [contentHeight, height, rowCount, rowHeight, updateScrollHint]);

  const zoomRowsBy = (dir: number) => {
    onRowHeightChange((h) => Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, h + dir * 4)));
  };

  const zoomRowsAll = () => {
    if (rowCount <= 0) return;
    const usableHeight = Math.max(1, height - topPad - GAP);
    onRowHeightChange(Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, Math.floor(usableHeight / rowCount))));
  };

  const onRowsWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (!event.altKey) return;
    event.preventDefault();
    event.stopPropagation();
    onRowHeightChange((h) => Math.min(MAX_ROW_HEIGHT, Math.max(MIN_ROW_HEIGHT, h - event.deltaY * 0.04)));
  };

  return (
    <div className="relative h-full w-full">
      <div
        className="absolute right-2 z-20 flex items-center gap-0.5 rounded border bg-white/90 px-1 py-0.5 shadow-sm"
        style={{ top: controlsTop }}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={(event) => event.stopPropagation()}
      >
        <span className="select-none px-0.5 text-[10px] font-medium text-muted-foreground">rows</span>
        <button type="button" className={ZOOM_BTN_CLASS} title="Shorter rows (Alt+scroll)" onClick={() => zoomRowsBy(-1)}>
          <Minus className="h-4 w-4" />
        </button>
        <button type="button" className={ZOOM_BTN_CLASS} title="Taller rows (Alt+scroll)" onClick={() => zoomRowsBy(1)}>
          <Plus className="h-4 w-4" />
        </button>
        <button type="button" className={`${ZOOM_BTN_CLASS} px-1 text-[10px] font-medium`} title="Fit all rows" onClick={zoomRowsAll}>
          all
        </button>
      </div>
      <div
        ref={scrollRef}
        className="h-full w-full overflow-y-auto overflow-x-hidden [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        onScroll={updateScrollHint}
        onWheel={onRowsWheel}
      >
        {children}
      </div>
      {scrollHint.canUp && (
        <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex h-14 items-start justify-center bg-gradient-to-b from-white via-white/70 to-transparent">
          <button
            type="button"
            className="pointer-events-auto mt-1.5 flex items-center gap-1 rounded-full border bg-white/95 px-2.5 py-1 text-[11px] font-medium text-muted-foreground shadow-sm transition-colors hover:text-primary"
            title="Scroll up to see earlier task rows"
            onPointerDown={(event) => event.stopPropagation()}
            onClick={() => scrollRef.current?.scrollBy({ top: -Math.max(1, height * 0.8), behavior: "smooth" })}
          >
            <ChevronUp className="h-3.5 w-3.5" />
            {scrollHint.hiddenAbove} more row{scrollHint.hiddenAbove === 1 ? "" : "s"} above
          </button>
        </div>
      )}
      {scrollHint.canDown && (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 flex h-14 items-end justify-center bg-gradient-to-t from-white via-white/70 to-transparent">
          <button
            type="button"
            className="pointer-events-auto mb-1.5 flex items-center gap-1 rounded-full border bg-white/95 px-2.5 py-1 text-[11px] font-medium text-muted-foreground shadow-sm transition-colors hover:text-primary"
            title="Scroll down to see more task rows"
            onPointerDown={(event) => event.stopPropagation()}
            onClick={() => scrollRef.current?.scrollBy({ top: Math.max(1, height * 0.8), behavior: "smooth" })}
          >
            <ChevronDown className="h-3.5 w-3.5" />
            {scrollHint.hiddenBelow} more row{scrollHint.hiddenBelow === 1 ? "" : "s"} below
          </button>
        </div>
      )}
    </div>
  );
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

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [colorMode, setColorMode] = useState<ColorMode>("kind-what");
  const [highlightedKey, setHighlightedKey] = useState<string | null>(null);
  const [hoveredUsageTime, setHoveredUsageTime] = useState<number | null>(null);
  const [hoveredWaitTime, setHoveredWaitTime] = useState<number | null>(null);
  const [highlightedResourceReason, setHighlightedResourceReason] = useState<string | null>(null);
  const [usageRowHeight, setUsageRowHeight] = useState(ROW_HEIGHT);
  const [waitRowHeight, setWaitRowHeight] = useState(ROW_HEIGHT);

  const { data, loading } = useResourceBlocking(what, dataRange.startTime, dataRange.endTime, numBins);
  const { data: usageData, loading: usageLoading } = useComponentTimeline(
    what,
    dataRange.startTime,
    dataRange.endTime,
    numBins,
    colorMode,
    true,
  );
  const dataMatchesRange =
    !!data &&
    sameTime(data.start_time, dataRange.startTime) &&
    sameTime(data.end_time, dataRange.endTime) &&
    data.num_bins === numBins;
  const usageDataMatchesRange =
    !!usageData &&
    sameTime(usageData.start_time, dataRange.startTime) &&
    sameTime(usageData.end_time, dataRange.endTime) &&
    usageData.num_bins === numBins;
  const [showGantt, setShowGantt] = useState(false);
  useEffect(() => {
    setShowGantt(false);
  }, [what]);
  useEffect(() => {
    if (dataMatchesRange) setShowGantt(!!data && data.total > 0 && data.total <= WAIT_GANTT_THRESHOLD);
  }, [dataMatchesRange, data?.total]);
  const showWaitGantt = showGantt;
  const showUsageGantt =
    usageDataMatchesRange &&
    (usageData?.sample ?? Infinity) === 1 &&
    (usageData?.total ?? Infinity) > 0 &&
    (usageData?.total ?? Infinity) <= USAGE_GANTT_THRESHOLD;
  const showAnyGantt = showUsageGantt || showWaitGantt;
  const waitTaskFetchEnabled = dataMatchesRange && showWaitGantt;
  const usageTaskFetchEnabled =
    usageDataMatchesRange &&
    showUsageGantt &&
    (usageData?.sample ?? Infinity) === 1 &&
    (usageData?.total ?? Infinity) > 0 &&
    (usageData?.total ?? Infinity) <= USAGE_GANTT_THRESHOLD;
  const { tasks: fetchedWaitTasks, loading: waitTasksLoading } = useResourceTasks(
    what,
    dataRange.startTime,
    dataRange.endTime,
    waitTaskFetchEnabled,
    WAIT_GANTT_THRESHOLD,
  );
  const usageTaskQuery = useMemo(
    () =>
      usageTaskFetchEnabled
        ? { where: what, startTime: dataRange.startTime, endTime: dataRange.endTime }
        : {},
    [dataRange.endTime, dataRange.startTime, usageTaskFetchEnabled, what],
  );
  const { tasks: fetchedUsageTasks, loading: usageTasksLoading } = useTraceData(usageTaskQuery);
  const [waitTasks, setWaitTasks] = useState<Task[]>([]);
  const [usageTasks, setUsageTasks] = useState<Task[]>([]);
  useEffect(() => {
    setWaitTasks([]);
    setUsageTasks([]);
  }, [what]);
  useEffect(() => {
    if (waitTaskFetchEnabled && !waitTasksLoading) setWaitTasks(fetchedWaitTasks);
  }, [fetchedWaitTasks, waitTaskFetchEnabled, waitTasksLoading]);
  useEffect(() => {
    if (dataMatchesRange && !waitTaskFetchEnabled) setWaitTasks([]);
  }, [dataMatchesRange, waitTaskFetchEnabled]);
  useEffect(() => {
    if (usageTaskFetchEnabled && !usageTasksLoading) setUsageTasks(fetchedUsageTasks);
  }, [fetchedUsageTasks, usageTaskFetchEnabled, usageTasksLoading]);
  useEffect(() => {
    if (usageDataMatchesRange && !usageTaskFetchEnabled) setUsageTasks([]);
  }, [usageDataMatchesRange, usageTaskFetchEnabled]);

  // Tasks located at the resource are the work that consumes it (e.g. inst-VALU
  // tasks at GPU[..].VALU). Other tasks carrying the resource milestone are the
  // higher-level tasks blocked by it (e.g. wavefront tasks).
  const blockedTasks = useMemo(
    () =>
      waitTasks.filter(
        (task) =>
          task.location !== what &&
          blockedIntervals(task, what).some(
            (interval) => interval.hi > viewRange.startTime && interval.lo < viewRange.endTime,
          ),
      ),
    [viewRange.endTime, viewRange.startTime, waitTasks, what],
  );
  const allTasks = useMemo(() => uniqueTasks(usageTasks, blockedTasks), [usageTasks, blockedTasks]);
  const selectedTask = allTasks.find((t) => String(t.id) === selectedId) ?? null;
  const usageLayout = useMemo(() => packTasksUp(usageTasks), [usageTasks]);
  const blockedLayout = useMemo(() => packTasksUp(blockedTasks), [blockedTasks]);
  const taskKeys = useMemo(() => {
    const keys = new Set<string>();
    for (const task of allTasks) keys.add(taskColorKey(task, colorMode));
    return Array.from(keys).sort();
  }, [allTasks, colorMode]);
  const handleColorMode = useAutoColorMode(colorMode, setColorMode, taskKeys.length, 10);
  const taskColorMap = useColorMapFromKeys(taskKeys, "task");
  const resourceReason = what ? taskColorKey({ kind: HW_RESOURCE_KIND, what }, "kind-what") : "";
  const resourceReasonColorMap = useMemo(
    () => (resourceReason ? { [resourceReason]: STROKE } : {}),
    [resourceReason],
  );
  const reasonHighlight = highlightedResourceReason ?? (hoveredWaitTime != null ? resourceReason : null);
  const highlightedUsageTaskIds = useMemo(() => {
    if (hoveredUsageTime == null || !showUsageGantt) return null;
    const ids = new Set<string>();
    for (const task of usageTasks) {
      if (task.start_time <= hoveredUsageTime && task.end_time >= hoveredUsageTime) ids.add(String(task.id));
    }
    return ids;
  }, [hoveredUsageTime, showUsageGantt, usageTasks]);
  const highlightedBlockedTaskIds = useMemo(() => {
    if (hoveredWaitTime == null || !showWaitGantt) return null;
    const ids = new Set<string>();
    for (const task of blockedTasks) {
      if (task.start_time > hoveredWaitTime || task.end_time < hoveredWaitTime) continue;
      if (isBlockedOnResourceAt(task, what, hoveredWaitTime)) ids.add(String(task.id));
    }
    return ids;
  }, [blockedTasks, hoveredWaitTime, showWaitGantt, what]);

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
  const resourceLabel = shortResourceLabel(what);

  // Vertical layout:
  // [tasks using resource gantt] [tasks using resource area]
  // [tasks that waited gantt] [tasks waiting area].
  // The two area curves are the aggregate counterparts of the two per-task panes.
  const areaRegionHeight = showAnyGantt ? Math.min(Math.max(80, Math.round(height * 0.16)), 150) : Math.max(1, Math.round(height * 0.5));
  const usageCurveRegionHeight = areaRegionHeight;
  const waitCurveRegionHeight = showAnyGantt ? areaRegionHeight : Math.max(1, height - usageCurveRegionHeight);
  const ganttRegionHeight = showAnyGantt ? Math.max(1, height - usageCurveRegionHeight - waitCurveRegionHeight) : 0;
  const taskGridTop = AXIS_PAD;
  const taskTop = taskGridTop;
  const blockingGridTop = 0;
  const blockingLabelTop = 16;
  const blockingTaskTop = 20;
  // Split the gantt region between the two panes. Start from an even split, but
  // when one pane's rows need less than its half (rows render at a fixed height,
  // so a few rows leave the rest blank), hand the surplus to the other pane.
  const usagePaneNeed = taskTop + GAP + usageLayout.rows * usageRowHeight;
  const waitPaneNeed = blockingTaskTop + GAP + blockedLayout.rows * waitRowHeight;
  const evenSplit = Math.round(ganttRegionHeight * 0.5);
  let taskRegionHeight = 0;
  let blockingRegionHeight = 0;
  if (showUsageGantt && showWaitGantt) {
    if (usagePaneNeed < evenSplit) {
      taskRegionHeight = Math.max(1, usagePaneNeed);
    } else if (waitPaneNeed < evenSplit) {
      taskRegionHeight = Math.max(1, Math.min(usagePaneNeed, ganttRegionHeight - waitPaneNeed));
    } else {
      taskRegionHeight = Math.max(1, evenSplit);
    }
    blockingRegionHeight = Math.max(1, ganttRegionHeight - taskRegionHeight);
  } else if (showUsageGantt) {
    taskRegionHeight = Math.max(1, ganttRegionHeight);
  } else if (showWaitGantt) {
    blockingRegionHeight = Math.max(1, ganttRegionHeight);
  }
  const usageCurveGridTop = showAnyGantt ? 0 : AXIS_PAD;
  const usageCurveGridBottom = Math.max(usageCurveGridTop + 1, usageCurveRegionHeight - AXIS_PAD);
  const waitCurveGridTop = showAnyGantt ? 0 : AXIS_PAD;
  const waitCurveGridBottom = Math.max(waitCurveGridTop + 1, waitCurveRegionHeight - AXIS_PAD);
  const taskContentHeight = showUsageGantt
    ? Math.max(taskRegionHeight, taskTop + GAP + usageLayout.rows * usageRowHeight)
    : 0;
  const blockingContentHeight = showWaitGantt
    ? Math.max(blockingRegionHeight, blockingTaskTop + GAP + blockedLayout.rows * waitRowHeight)
    : 0;
  const taskGridBottom = Math.max(taskGridTop + 1, taskContentHeight);
  const blockingGridBottom = Math.max(1, blockingContentHeight);

  const usageBins = useMemo(
    () => (usageDataMatchesRange ? componentBinsTotal(usageData?.bins ?? []) : []),
    [usageData?.bins, usageDataMatchesRange],
  );
  const usageTotalLabel = usageDataMatchesRange && usageData ? usageData.total.toLocaleString() : "loading";
  const usageSample = usageDataMatchesRange && usageData ? usageData.sample ?? 1 : 1;

  const { areaPath: usageAreaPath, yScale: usageYScale } = useMemo(() => {
    const bins = usageBins;
    const maxV = Math.max(1, d3.max(bins) ?? 1);
    const y = d3.scaleLinear().domain([0, maxV]).nice().range([usageCurveGridBottom, usageCurveGridTop + CURVE_PAD_TOP]);
    const dStart = usageData?.start_time ?? startTime;
    const n = usageData?.num_bins ?? bins.length;
    const binW = n > 0 ? ((usageData?.end_time ?? endTime) - dStart) / n : 0;
    const pts = bins.map((v, b) => ({ t: dStart + (b + 0.5) * binW, v }));
    const area = d3
      .area<{ t: number; v: number }>()
      .x((p) => safeScale(xScale, p.t))
      .y0(safeScale(y, 0))
      .y1((p) => safeScale(y, p.v))
      .curve(d3.curveMonotoneX);
    return { areaPath: area(pts) ?? "", yScale: y };
  }, [usageBins, usageData, xScale, startTime, endTime, usageCurveGridTop, usageCurveGridBottom]);

  const { areaPath: waitAreaPath, yScale: waitYScale } = useMemo(() => {
    const bins = data?.bins ?? [];
    const maxV = Math.max(1, d3.max(bins) ?? 1);
    // Leave CURVE_PAD_TOP at the top of the band for the label, matching the
    // component page's task-count area (padTop above the peak).
    const y = d3.scaleLinear().domain([0, maxV]).nice().range([waitCurveGridBottom, waitCurveGridTop + CURVE_PAD_TOP]);
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
  }, [data, xScale, startTime, endTime, waitCurveGridTop, waitCurveGridBottom]);

  const gaps = segmentsData?.enabled ? gapSegments(segmentsData.segments, startTime, endTime) : [];
  const hasData = (data?.bins.length ?? 0) > 0;
  // Row height is controlled only by the rows -/+/all control; time zoom must not
  // change it (dividing the region by the visible row count made Ctrl/Cmd+scroll
  // fatten the rows as tasks left the range).
  const rowH = showUsageGantt && usageLayout.rows > 0 ? usageRowHeight : 0;
  const blockingRowH = showWaitGantt && blockedLayout.rows > 0 ? waitRowHeight : 0;
  const chartUpdating =
    what &&
    ((!hasData && loading) ||
      (showWaitGantt && waitTaskFetchEnabled && waitTasks.length === 0) ||
      (showUsageGantt && usageTaskFetchEnabled && usageTasks.length === 0));

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
          {(dataPending ||
            loading ||
            usageLoading ||
            waitTasksLoading ||
            usageTasksLoading ||
            (what && data && !dataMatchesRange) ||
            (what && usageData && !usageDataMatchesRange)) &&
          what ? (
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
        {/* What the view means lives in the chart-corner info buttons; the panel
            itself just reflects the selected task and legends. */}
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
          setHoveredUsageTime(null);
          setHoveredWaitTime(null);
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
              {showUsageGantt ? (
                <div className="daisen1-component-view relative" style={{ height: taskRegionHeight }}>
                  <ResourceGanttScrollFrame
                    height={taskRegionHeight}
                    contentHeight={taskContentHeight}
                    rowHeight={usageRowHeight}
                    rowCount={usageLayout.rows}
                    topPad={taskTop}
                    controlsTop={36}
                    onRowHeightChange={setUsageRowHeight}
                  >
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
                      const hasHighlight = highlightedUsageTaskIds !== null || highlightedKey !== null;
                      const highlighted =
                        highlightedUsageTaskIds !== null
                          ? highlightedUsageTaskIds.has(String(task.id))
                          : highlightedKey !== null
                            ? highlightedKey === key
                            : true;
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
                        {`Tasks using ${resourceLabel} · ${usageTasks.length.toLocaleString()} tasks`}
                      </text>
                    </svg>
                  </ResourceGanttScrollFrame>
                  <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
                    <ResourceUsageTasksHelp className={CHART_HELP_BUTTON} />
                  </div>
                </div>
              ) : null}
              <div
                className={`daisen1-resource-usage-curve relative${showAnyGantt ? " border-t border-slate-200" : ""}`}
                style={{ height: usageCurveRegionHeight }}
              >
                <svg
                  width={width}
                  height={usageCurveRegionHeight}
                  className="block"
                  onMouseMove={(event) => {
                    const rect = event.currentTarget.getBoundingClientRect();
                    setHoveredUsageTime(xScale.invert(event.clientX - rect.left));
                  }}
                  onMouseLeave={() => setHoveredUsageTime(null)}
                >
                  <TimeTicks
                    ticks={xScale.ticks(AXIS_TICK_COUNT)}
                    xScale={xScale}
                    gridTop={usageCurveGridTop}
                    gridBottom={usageCurveGridBottom}
                    topLabelY={showAnyGantt ? undefined : 12}
                    tickMarks={!showAnyGantt}
                  />
                  {!showAnyGantt ? <line x1={5} x2={width - 5} y1={usageCurveGridTop} y2={usageCurveGridTop} stroke={COLOR_GRID} /> : null}
                  <line x1={5} x2={width - 5} y1={usageCurveGridBottom} y2={usageCurveGridBottom} stroke={COLOR_GRID} />
                  {usageDataMatchesRange ? (
                    <path d={usageAreaPath} fill={taskColorMap[taskColorKey({ kind: "inst", what: resourceLabel }, "kind-what")] ?? "#8b5cf6"} opacity={0.85} />
                  ) : (
                    <LoadingCurve width={width} height={usageCurveRegionHeight} id="resource-usage" />
                  )}
                  <YAxisOverlay yScale={usageYScale} width={width} />
                  <text
                    x={8}
                    y={usageCurveGridTop + 13}
                    fontSize="11"
                    fill="#475569"
                    stroke="#ffffff"
                    strokeWidth={2.5}
                    paintOrder="stroke"
                    pointerEvents="none"
                  >
                    {`Tasks using ${resourceLabel} over time · ${usageTotalLabel} tasks${
                      usageSample > 1 ? ` · ≈1-in-${usageSample} sample` : ""
                    }${usageDataMatchesRange && usageData && !showUsageGantt && usageData.total > 0 ? " · zoom in for individual tasks" : ""}`}
                  </text>
                  <GapShading gaps={gaps} xScale={xScale} height={usageCurveGridBottom} patternId="resource-usage-gap" />
                </svg>
                <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
                  <ResourceUsageCountHelp className={CHART_HELP_BUTTON} />
                </div>
              </div>
              {showWaitGantt ? (
                <div className="daisen1-resource-blocking-view relative border-t border-slate-200" style={{ height: blockingRegionHeight }}>
                  <ResourceGanttScrollFrame
                    height={blockingRegionHeight}
                    contentHeight={blockingContentHeight}
                    rowHeight={waitRowHeight}
                    rowCount={blockedLayout.rows}
                    topPad={blockingTaskTop}
                    onRowHeightChange={setWaitRowHeight}
                  >
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
                            const showRelease = iv.hi >= startTime && iv.hi <= endTime;
                            const showWave = x1 - x0 >= 1;
                            if (!showWave && !showRelease) return null;
                            return (
                              <g key={`${task.id}-${k}`}>
                                {showWave ? (
                                  <>
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
                                      stroke={COLOR_HALO}
                                      strokeWidth={Math.max(3, MILESTONE_WAVE_WIDTH + 2)}
                                      strokeLinecap="round"
                                      opacity={opacity}
                                      pointerEvents="none"
                                    />
                                    <path
                                      d={wavyPath(x0, x1, centerY, amplitude, 3)}
                                      fill="none"
                                      stroke={STROKE}
                                      strokeWidth={Math.max(1.5, MILESTONE_WAVE_WIDTH)}
                                      strokeLinecap="round"
                                      opacity={opacity}
                                      pointerEvents="none"
                                    />
                                  </>
                                ) : null}
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
                        {`Tasks that waited for ${resourceLabel} · ${blockedTasks.length.toLocaleString()} tasks`}
                      </text>
                    </svg>
                  </ResourceGanttScrollFrame>
                  <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
                    <ResourceWaitTasksHelp className={CHART_HELP_BUTTON} />
                  </div>
                </div>
              ) : null}
              <div
                className={`daisen1-metric-view relative${showAnyGantt ? " border-t border-slate-200" : ""}`}
                style={{ height: waitCurveRegionHeight }}
              >
                <svg
                  width={width}
                  height={waitCurveRegionHeight}
                  className="block"
                  onMouseMove={(event) => {
                    const rect = event.currentTarget.getBoundingClientRect();
                    setHoveredWaitTime(xScale.invert(event.clientX - rect.left));
                  }}
                  onMouseLeave={() => setHoveredWaitTime(null)}
                >
                  <TimeTicks
                    ticks={xScale.ticks(AXIS_TICK_COUNT)}
                    xScale={xScale}
                    gridTop={waitCurveGridTop}
                    gridBottom={waitCurveGridBottom}
                    topLabelY={showAnyGantt ? undefined : 12}
                    bottomLabelY={waitCurveRegionHeight - 6}
                    tickMarks
                  />
                  {!showAnyGantt ? <line x1={5} x2={width - 5} y1={waitCurveGridTop} y2={waitCurveGridTop} stroke={COLOR_GRID} /> : null}
                  <line x1={5} x2={width - 5} y1={waitCurveGridBottom} y2={waitCurveGridBottom} stroke={COLOR_GRID} />

                  {/* Occupancy curve (always) — a filled band only, matching the
                      component page's task-count area (no outline, 0.9 opacity). */}
                  <path d={waitAreaPath} fill={FILL} opacity={0.9} />
                  <YAxisOverlay yScale={waitYScale} width={width} />
                  <text
                    x={8}
                    y={waitCurveGridTop + 13}
                    fontSize="11"
                    fill="#475569"
                    stroke="#ffffff"
                    strokeWidth={2.5}
                    paintOrder="stroke"
                    pointerEvents="none"
                  >
                    {`Tasks waiting for ${resourceLabel} over time · ${data?.total.toLocaleString() ?? "—"} tasks${
                      data && data.sample > 1 ? ` · ≈1-in-${data.sample} sample` : ""
                    }${data && !showWaitGantt && data.total > 0 ? " · zoom in for individual tasks" : ""}`}
                  </text>

                  <GapShading gaps={gaps} xScale={xScale} height={waitCurveGridBottom} patternId="resource-curve-gap" />
                </svg>
                <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()} onClick={(e) => e.stopPropagation()}>
                  <ResourceWaitCountHelp className={CHART_HELP_BUTTON} />
                </div>
              </div>
            </>
          )}
        </div>

        <TimeZoomControls onZoom={(dir) => zoomBy(dir > 0 ? 1.4 : 0.7)} className="absolute right-2 top-1" />
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
