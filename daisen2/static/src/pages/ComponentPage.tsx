import * as d3 from "d3";
import { useEffect, useMemo, useRef, useState } from "react";
import type { MouseEvent as ReactMouseEvent, PointerEvent, WheelEvent as ReactWheelEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { X, ChevronRight, ChevronDown, ChevronUp, Plus, Minus } from "lucide-react";
import { Button } from "../components/ui/button";
import { SidePanel } from "../components/ui/side-panel";
import { BlockingReasonsHelp, ComponentTaskViewHelp, TaskCountHelp, TaskHierarchyHelp } from "../components/HelpTopics";
import Legend from "../components/Legend";
import TaskDetail from "../components/TaskDetail";
import type { StackedComponentInfo } from "../hooks/useCompInfo";
import { useStackedCompInfo } from "../hooks/useCompInfo";
import { useSegments } from "../hooks/useSegments";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useTraceData } from "../hooks/useTraceData";
import { useComponentTimeline } from "../hooks/useComponentTimeline";
import type { ComponentTimelineData } from "../hooks/useComponentTimeline";
import { useRenderReady } from "../hooks/useRenderReady";
import type { Segment, Task } from "../types/task";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../utils/taskColorCoder";
import type { ColorMode } from "../utils/taskColorCoder";
import { blockingKindAt, milestonesOf, wavyPath } from "../utils/milestoneViz";
import { smartString } from "../utils/smartValue";
import { cn } from "../lib/utils";
import { useComponentNames } from "../hooks/useComponentNames";
import { buildLocationTree, breadcrumbSegments, findNode, type LocationNode } from "../utils/locationTree";

// The left column stacks three regions: the parent/current/sub task view (top),
// the component-task timeline (middle), and the metric line chart (bottom). The
// task view and the metric line each take a fixed share of the window height; the
// timeline fills the rest. In component mode (no task selected) the task view
// collapses to a thin time axis so the timeline gets that space too.
const TASK_VIEW_HEIGHT_RATIO = 0.2;
const COMPONENT_LINE_HEIGHT_RATIO = 0.2;
const TOP_AXIS_COMPACT_HEIGHT = 28;
const SIDE_COLUMN_WIDTH = 350;
const DATA_RANGE_DEBOUNCE_MS = 1000;
// A help button tucked into a chart region's bottom-right corner. The wrapper stops
// pointer events so clicking it never starts a pan/drag on the timeline underneath;
// the button gets a translucent background so it stays legible over the chart.
const CHART_HELP_CORNER = "absolute bottom-2 right-2 z-20";
const CHART_HELP_BUTTON =
  "bg-white/85 p-1 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm hover:bg-white";
// Above this many tasks in the visible range, the per-task timeline (one SVG
// element per task) becomes the page's dominant cost, so we switch to the
// server-aggregated density view instead. Zooming in until the count drops below
// the threshold brings the individual, interactive task bars back.
const RAW_TASK_THRESHOLD = 5000;
// Fixed height of one concurrency row in the per-task gantt. Rows no longer share
// a divided region (which made bars 1px when many overlapped); the chart grows
// past its region and is navigated by dragging / scroll buttons instead.
const ROW_HEIGHT = 22;

const MIN_RANGE = 1e-12;
const TASK_VIEW_MARGIN_TOP = 20;
const TASK_VIEW_MARGIN_BOTTOM = 20;
const TASK_VIEW_GROUP_GAP = 10;
const TASK_VIEW_LARGE_TASK_HEIGHT = 15;
// Height of one subtask row in the task view (the cap used when laying out the
// subtask bars). Also drives the dynamic task-view height so the region shrinks to
// the number of subtask rows instead of leaving a fixed empty band.
const TASK_VIEW_SUBTASK_BAR_HEIGHT = 10;
// Vertical room reserved below the Current Task bar for the blocking-reason
// wavy lines (only when the task has milestones).
const TASK_VIEW_MILESTONE_BAND = 18;
// Vertical strip (px) reserved at the top and bottom of a parent task so it
// stays visible behind sub-tasks that span its full duration.
const NEST_PAD = 3;

interface TimeRange {
  startTime: number;
  endTime: number;
}

interface Size {
  width: number;
  height: number;
}

interface TaskDim {
  x: number;
  y: number;
  width: number;
  height: number;
  startTime: number;
  endTime: number;
}

interface TaskViewRow {
  task: Task;
  x: number;
  y: number;
  width: number;
  height: number;
}

type LayoutTask = Task & {
  subTasks: LayoutTask[];
  level: number;
  // Subtree end: the latest end_time over this task and all its descendants.
  // Concurrency packing uses this (not the task's own end) so a parent reserves
  // its row for an out-running child — e.g. an L2 writeback that outlives the
  // write request that spawned it — and nothing unrelated is packed into the row
  // space that child draws over.
  effEnd?: number;
  dim?: TaskDim;
};

function useElementSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  const [size, setSize] = useState<Size>({ width: 1000, height: 700 });

  useEffect(() => {
    if (!ref.current) return;
    const observer = new ResizeObserver(([entry]) => {
      setSize({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      });
    });
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);

  return { ref, size };
}

function useDebouncedValue<T>(value: T, delayMs: number) {
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setDebouncedValue(value);
    }, delayMs);

    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);

  return debouncedValue;
}

function safeScale(scale: d3.ScaleLinear<number, number>, value: number) {
  return scale(value) ?? 0;
}

function formatAxisTick(value: number) {
  return d3.format("~s")(value);
}

function gapSegments(segments: Segment[], startTime: number, endTime: number) {
  if (!segments.length) return [];
  const sorted = [...segments].sort((a, b) => a.start_time - b.start_time);
  const gaps: Segment[] = [];
  if (sorted[0].start_time > startTime) {
    gaps.push({ start_time: startTime, end_time: Math.min(sorted[0].start_time, endTime) });
  }
  for (let index = 0; index < sorted.length - 1; index++) {
    const start = Math.max(sorted[index].end_time, startTime);
    const end = Math.min(sorted[index + 1].start_time, endTime);
    if (start < end) gaps.push({ start_time: start, end_time: end });
  }
  const last = sorted[sorted.length - 1];
  if (last.end_time < endTime) {
    gaps.push({ start_time: Math.max(last.end_time, startTime), end_time: endTime });
  }
  return gaps;
}

function cloneTasks(tasks: Task[]): LayoutTask[] {
  return tasks.map((task) => ({ ...task, subTasks: [], level: 0 }));
}

function buildTaskTree(tasks: LayoutTask[]) {
  const root: LayoutTask = {
    id: "__root__",
    parent_id: "",
    kind: "",
    what: "",
    location: "",
    start_time: 0,
    end_time: 0,
    subTasks: [],
    level: 0,
  };
  const taskById = new Map<string | number, LayoutTask>();

  for (const task of tasks) {
    task.subTasks = [];
    taskById.set(task.id, task);
  }

  for (const task of tasks) {
    const parent = taskById.get(task.parent_id);
    if (parent) {
      parent.subTasks.push(task);
    } else {
      root.subTasks.push(task);
    }
  }

  for (const task of root.subTasks) {
    assignTaskLevel(task, 1);
  }
  computeEffectiveEnd(root);

  return root;
}

function assignTaskLevel(task: LayoutTask, level: number) {
  task.level = level;
  for (const child of task.subTasks) {
    assignTaskLevel(child, level + 1);
  }
}

// computeEffectiveEnd sets each task's effEnd to the latest end_time in its
// subtree (itself plus all descendants), bottom-up, so concurrency packing can
// reserve a row for a task's whole subtree rather than just its own span.
function computeEffectiveEnd(task: LayoutTask): number {
  let end = task.end_time;
  for (const child of task.subTasks) {
    end = Math.max(end, computeEffectiveEnd(child));
  }
  task.effEnd = end;

  return end;
}

function tasksAtLevel(task: LayoutTask, depth: number, output: LayoutTask[]) {
  if (depth === 0) {
    output.push(task);
    return;
  }

  for (const child of task.subTasks) {
    tasksAtLevel(child, depth - 1, output);
  }
}

function assignYIndices(tasks: LayoutTask[]) {
  const rows: LayoutTask[][] = [];
  let maxYIndex = -1;
  tasks.sort((a, b) => a.start_time - b.start_time);

  for (const task of tasks) {
    let index = 0;
    while (hasConflict(task, rows[index])) {
      index++;
    }
    if (rows.length === index) {
      rows.push([]);
    }
    rows[index].push(task);
    task.yIndex = index;
    maxYIndex = Math.max(maxYIndex, index);
  }

  return maxYIndex;
}

function hasConflict(task: LayoutTask, row?: LayoutTask[]) {
  if (!row) return false;

  // Pack by the subtree span [start_time, effEnd], not the task's own end, so a
  // task reserves room for an out-running descendant and nothing unrelated is
  // placed in the row space that descendant draws over.
  const taskEnd = task.effEnd ?? task.end_time;

  return row.some((other) => {
    const otherEnd = other.effEnd ?? other.end_time;
    if (other.start_time <= task.start_time && otherEnd > task.start_time) return true;
    if (other.start_time < taskEnd && otherEnd >= taskEnd) return true;
    if (task.start_time <= other.start_time && taskEnd >= otherEnd) return true;
    if (task.start_time >= other.start_time && taskEnd <= otherEnd) return true;
    return false;
  });
}

function padTaskHeight(height: number) {
  // Fill most of the row, leaving a small vertical gap between stacked bars. The
  // gap shrinks to nothing as the rows get tight (dense concurrency) so the bars
  // stay visible instead of vanishing into the gap. Subtasks nest within the
  // returned band, so this is the parent bar's height, not half of it.
  const gap = Math.min(4, Math.max(0, (height - 6) * 0.5));
  return Math.max(1, height - gap);
}

function assignDimensionLevel(root: LayoutTask, parentLevelHeight: number, depth: number) {
  if (parentLevelHeight < 2) return 0;

  const levelTasks: LayoutTask[] = [];
  tasksAtLevel(root, depth, levelTasks);
  if (!levelTasks.length) return 0;

  let globalMaxY = -1;
  for (const task of levelTasks) {
    globalMaxY = Math.max(globalMaxY, assignYIndices(task.subTasks));
  }

  if (globalMaxY === -1) return 0;

  // Reserve a strip at the top and bottom of each parent task so it stays
  // visible behind sub-tasks that span its full duration. depth 0's "parent" is
  // the synthetic, undrawn root, so it needs no strip; the strip also shrinks on
  // tight bands so the sub-tasks don't vanish.
  const nestPad = depth === 0 ? 0 : Math.min(NEST_PAD, Math.max(0, (parentLevelHeight - 4) * 0.25));
  const childBand = parentLevelHeight - 2 * nestPad;

  const taskHeight = childBand / (globalMaxY + 1);
  const paddedTaskHeight = padTaskHeight(taskHeight);

  for (const task of levelTasks) {
    if (!task.dim) continue;
    for (const child of task.subTasks) {
      const pixelPerSecond = task.dim.width / Math.max(MIN_RANGE, task.dim.endTime - task.dim.startTime);
      const duration = child.end_time - child.start_time;
      const offsetDuration = child.start_time - task.dim.startTime;
      child.dim = {
        startTime: child.start_time,
        endTime: child.end_time,
        height: paddedTaskHeight,
        y: task.dim.y + nestPad + taskHeight * (child.yIndex ?? 0) + (taskHeight - paddedTaskHeight) / 2,
        width: duration * pixelPerSecond,
        x: offsetDuration * pixelPerSecond + task.dim.x,
      };
    }
  }

  return paddedTaskHeight;
}

function assignDimensions(root: LayoutTask, initialDim: TaskDim) {
  let taskHeight = initialDim.height;
  let depth = 0;
  root.dim = initialDim;
  while (taskHeight > 0) {
    taskHeight = assignDimensionLevel(root, taskHeight, depth);
    depth++;
  }
}

function buildComponentTaskLayout(
  tasks: Task[],
  width: number,
  regionHeight: number,
  startTime: number,
  endTime: number,
  rowHeight: number,
) {
  const clonedTasks = cloneTasks(tasks);
  const root = buildTaskTree(clonedTasks);
  // Give each top-level concurrency row a fixed rowHeight so bars stay legible no
  // matter how many overlap; the chart then grows past its region and scrolls.
  // When few rows are needed, fall back to filling the region (no wasted space).
  const topRows = assignYIndices(root.subTasks) + 1;
  const contentHeight = Math.max(regionHeight, topRows * rowHeight);
  assignDimensions(root, {
    x: 0,
    y: 0,
    width,
    height: contentHeight,
    startTime,
    endTime,
  });

  const layout = clonedTasks
    .sort((a, b) => a.level - b.level)
    .filter((task) => {
      if (task.level === 1) return true;
      if (!task.dim) return false;
      if (task.dim.width < 1) return false;
      if (task.dim.height < 1) return false;
      return true;
    });

  return { layout, contentHeight, topRows };
}

function ComponentTopAxis({ width, height, range }: { width: number; height: number; range: TimeRange }) {
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);

  // Top axis: tick labels sit at the top (above the baseline), mirroring the bottom
  // chart's axis whose labels sit below its baseline. The baseline + ticks + the
  // gridlines hang below the labels, toward the content.
  return (
    <svg width={width} height={height} className="block">
      {ticks.map((tick) => (
        <g key={tick}>
          <text x={safeScale(xScale, tick)} y={11} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={16} y2={height} stroke="#000" strokeDasharray="3,3" opacity={0.5} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={16} y2={22} stroke="#000" />
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={16} y2={16} stroke="#000" />
    </svg>
  );
}

// Zoom toolbar button styling, shared by the global time-zoom control and the
// gantt's row-zoom control so both toolbars read identically.
const ZOOM_BTN_CLASS = "rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-primary";

// TimeZoomControls is the horizontal (time-axis) zoom widget. It is rendered once
// at the page level so time zoom is always available — independent of whether the
// per-task gantt is shown. onZoom(dir) zooms out for dir > 0 and in for dir < 0.
function TimeZoomControls({ onZoom, className }: { onZoom: (dir: number) => void; className?: string }) {
  return (
    <div
      className={cn(
        "z-10 flex items-center gap-0.5 rounded border bg-white/90 px-1 py-0.5 shadow-sm",
        className,
      )}
      // stopPropagation so a click on the toolbar doesn't reach the left column's
      // pan/drag handlers (which capture the pointer and would swallow the click).
      onPointerDown={(event) => event.stopPropagation()}
    >
      <span className="select-none px-0.5 text-[10px] font-medium text-muted-foreground">time</span>
      <button type="button" className={ZOOM_BTN_CLASS} title="Zoom time out" onClick={() => onZoom(1)}>
        <Minus className="h-4 w-4" />
      </button>
      <button type="button" className={ZOOM_BTN_CLASS} title="Zoom time in" onClick={() => onZoom(-1)}>
        <Plus className="h-4 w-4" />
      </button>
    </div>
  );
}

interface ComponentTimelineProps {
  name: string;
  tasks: Task[];
  segments: Segment[];
  segmentsEnabled: boolean;
  range: TimeRange;
  size: Size;
  colorMap: Record<string, string>;
  colorMode: ColorMode;
  highlightedKey: string | null;
  highlightedTaskId: string | null;
  highlightedTaskIds: Set<string> | null;
  selectedTaskId: string | null;
  onHoverTask: (task: Task | null) => void;
  onSelectTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onDeselect: () => void;
  // Zoom the time range from a wheel/pinch over the gantt. pointerRatio is the
  // cursor's fractional x within the chart, so the zoom is anchored at the pointer.
  onZoom: (deltaY: number, deltaX: number, pointerRatio: number) => void;
  // Set the visible time range (used by horizontal drag-panning).
  onRangeChange: (range: TimeRange) => void;
}

function ComponentTimeline({
  name,
  tasks,
  segments,
  segmentsEnabled,
  range,
  size,
  colorMap,
  colorMode,
  highlightedKey,
  highlightedTaskId,
  highlightedTaskIds,
  selectedTaskId,
  onHoverTask,
  onSelectTask,
  onOpenTask,
  onDeselect,
  onZoom,
  onRangeChange,
}: ComponentTimelineProps) {
  const width = Math.max(1, size.width);
  const height = Math.max(1, size.height);
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  // Row height is the vertical zoom: taller rows = bigger bars, a taller chart that
  // scrolls. The chart grows past its region and is navigated by dragging/scrolling.
  const [rowHeight, setRowHeight] = useState(ROW_HEIGHT);
  // Whether more task rows sit above/below the visible area (and roughly how many),
  // so we can show "scroll for more" affordances at each end. Both stay hidden when
  // everything fits; the top one also hides at the very top, the bottom at the end.
  const [scrollHint, setScrollHint] = useState({ canUp: false, hiddenAbove: 0, canDown: false, hiddenBelow: 0 });
  const { layout: taskLayout, contentHeight, topRows } = buildComponentTaskLayout(tasks, width, height, range.startTime, range.endTime, rowHeight);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];

  const taskById = useMemo(() => {
    const map = new Map<string, Task>();
    for (const task of tasks) map.set(String(task.id), task);
    return map;
  }, [tasks]);

  // Latest values for the imperative wheel / pointer handlers.
  const scrollRef = useRef<HTMLDivElement>(null);
  const onZoomRef = useRef(onZoom);
  onZoomRef.current = onZoom;
  const onRangeChangeRef = useRef(onRangeChange);
  onRangeChangeRef.current = onRangeChange;
  const onSelectTaskRef = useRef(onSelectTask);
  onSelectTaskRef.current = onSelectTask;
  const onOpenTaskRef = useRef(onOpenTask);
  onOpenTaskRef.current = onOpenTask;
  const onDeselectRef = useRef(onDeselect);
  onDeselectRef.current = onDeselect;
  // The pointer capture used for drag-panning swallows the bars' native dblclick,
  // so detect a double-click manually: two clicks on the same task in quick
  // succession open it in the task view.
  const lastClickRef = useRef<{ id: string; time: number } | null>(null);
  const rangeRef = useRef(range);
  rangeRef.current = range;
  const widthRef = useRef(width);
  widthRef.current = width;
  const taskByIdRef = useRef(taskById);
  taskByIdRef.current = taskById;

  // Wheel handling on a non-passive native listener (React's onWheel is passive, so
  // preventDefault is ignored there). Plain scroll PANS — horizontal moves the time
  // range, vertical scrolls the rows. Ctrl/⌘+scroll — and a trackpad pinch, which
  // the browser delivers as a ctrl+wheel — zooms the TIME axis (anchored at the
  // cursor). Row (vertical) zoom is on Alt+scroll, kept off ctrl so it never
  // collides with pinch. We always stopPropagation so the parent's region wheel
  // handler never double-fires.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return undefined;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      event.stopPropagation();
      const bounds = el.getBoundingClientRect();
      const ratio = bounds.width > 0 ? Math.min(Math.max(event.clientX - bounds.left, 0), bounds.width) / bounds.width : 0.5;
      if (event.altKey) {
        setRowHeight((h) => Math.min(80, Math.max(6, h - event.deltaY * 0.04)));
      } else if (event.ctrlKey || event.metaKey) {
        const delta = Math.abs(event.deltaY) >= Math.abs(event.deltaX) ? event.deltaY : event.deltaX;
        onZoomRef.current(delta, 0, ratio);
      } else {
        if (event.deltaX !== 0) onZoomRef.current(0, event.deltaX, ratio);
        if (event.deltaY !== 0) el.scrollTop += event.deltaY;
      }
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  // Recompute the scroll hint from the container: how far is left below the fold,
  // and roughly how many rows that is. Driven by the scroll handler and re-run when
  // the layout changes (row height / content height / region height / task count).
  const updateScrollHint = () => {
    const el = scrollRef.current;
    if (!el) return;
    const above = el.scrollTop;
    const below = el.scrollHeight - el.clientHeight - el.scrollTop;
    const rows = (px: number) => Math.max(0, Math.ceil(px / Math.max(1, rowHeight)));
    setScrollHint({ canUp: above > 1, hiddenAbove: rows(above), canDown: below > 1, hiddenBelow: rows(below) });
  };
  useEffect(() => {
    updateScrollHint();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [contentHeight, height, rowHeight, tasks.length]);

  // Drag pans in both directions: horizontal moves the time range, vertical scrolls
  // through the concurrency rows. A press with no movement selects the task.
  const dragRef = useRef<{ id: number; x: number; y: number; range: TimeRange; scrollTop: number; pending: Task | null; moved: boolean } | null>(null);
  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.stopPropagation();
    event.currentTarget.setPointerCapture(event.pointerId);
    let pending: Task | null = null;
    if (event.target instanceof Element) {
      const taskId = event.target.closest("[data-task-id]")?.getAttribute("data-task-id");
      pending = taskId ? (taskByIdRef.current.get(taskId) ?? null) : null;
    }
    dragRef.current = {
      id: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      range: rangeRef.current,
      scrollTop: scrollRef.current?.scrollTop ?? 0,
      pending,
      moved: false,
    };
  };
  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.id !== event.pointerId) return;
    event.stopPropagation();
    const dx = event.clientX - drag.x;
    const dy = event.clientY - drag.y;
    if (Math.abs(dx) > 2 || Math.abs(dy) > 2) drag.moved = true;
    const duration = drag.range.endTime - drag.range.startTime;
    const timeDelta = (duration / Math.max(1, widthRef.current)) * dx;
    onRangeChangeRef.current({ startTime: drag.range.startTime - timeDelta, endTime: drag.range.endTime - timeDelta });
    if (scrollRef.current) scrollRef.current.scrollTop = drag.scrollTop - dy;
  };
  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.id !== event.pointerId) return;
    event.stopPropagation();
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    dragRef.current = null;
    if (drag.moved) return;
    if (!drag.pending) {
      // A click that landed on empty space (no task) clears the selection.
      onDeselectRef.current();
      return;
    }

    const id = String(drag.pending.id);
    const now = Date.now();
    const last = lastClickRef.current;
    if (last && last.id === id && now - last.time < 350) {
      // Second quick click on the same task — open it in the task view.
      lastClickRef.current = null;
      onOpenTaskRef.current(drag.pending);
    } else {
      lastClickRef.current = { id, time: now };
      onSelectTaskRef.current(drag.pending);
    }
  };

  // On-screen vertical (row height) zoom controls. Horizontal/time zoom lives in
  // the always-on page-level toolbar (TimeZoomControls), since it applies to every
  // view; row zoom is specific to the per-task gantt and stays here. The gantt
  // still zooms time on Ctrl/⌘+scroll via onZoom (see the wheel handler above).
  const zoomRowsBy = (dir: number) => setRowHeight((h) => Math.min(80, Math.max(6, h + dir * 4)));
  // Shrink rows so every concurrency row fits in the visible region at once.
  const zoomRowsAll = () => setRowHeight(Math.max(2, Math.min(80, Math.floor(height / Math.max(1, topRows)))));

  return (
    <div className="relative h-full w-full">
      {/* stopPropagation so a click on the toolbar doesn't reach the gantt/parent
          drag handlers (which capture the pointer and would swallow the click). */}
      <div
        className="absolute right-2 top-1 z-20 flex items-center gap-0.5 rounded border bg-white/90 px-1 py-0.5 shadow-sm"
        onPointerDown={(event) => event.stopPropagation()}
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
        className="h-full w-full cursor-grab overflow-y-auto overflow-x-hidden active:cursor-grabbing"
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onScroll={updateScrollHint}
      >
        <svg width={width} height={contentHeight} className="block">
          <defs>
            <pattern id="component-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
              <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
              <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
            </pattern>
          </defs>

          {gaps.map((gap, index) => {
            const x = safeScale(xScale, gap.start_time);
            const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
            return <rect key={index} x={x} y={0} width={w} height={contentHeight} fill="url(#component-gap-pattern)" pointerEvents="none" />;
          })}

          <g className="task-bar cursor-pointer">
            {taskLayout.map((task) => {
          const dim = task.dim ?? {
            x: safeScale(xScale, task.start_time),
            y: 0,
            width: Math.max(1, safeScale(xScale, task.end_time) - safeScale(xScale, task.start_time)),
            height: 8,
          };
          const key = taskColorKey(task, colorMode);
          const taskHighlighted = highlightedTaskId === String(task.id);
          const keyHighlighted = highlightedKey === key;
          const hasHighlight = highlightedTaskIds !== null || highlightedTaskId !== null || highlightedKey !== null;
          const highlighted =
            highlightedTaskIds !== null
              ? highlightedTaskIds.has(String(task.id))
              : highlightedTaskId !== null
                ? taskHighlighted
                : highlightedKey !== null
                  ? keyHighlighted
                  : true;
          const selected = selectedTaskId != null && selectedTaskId === String(task.id);
          return (
            <rect
              key={String(task.id)}
              data-task-id={String(task.id)}
              x={dim.x}
              y={dim.y}
              width={Math.max(1, dim.width)}
              height={Math.max(1, dim.height)}
              fill={lookupColor(colorMap, task, colorMode)}
              stroke="#000000"
              strokeOpacity={selected || (hasHighlight && highlighted) ? 0.8 : 0.2}
              opacity={hasHighlight ? (highlighted ? 1 : 0.4) : selectedTaskId != null && !selected ? 0.6 : 1}
            >
              <title>
                {task.kind} - {task.what}
                {"\n"}
                {name}
                {"\n"}
                {smartString(task.start_time)} to {smartString(task.end_time)}
              </title>
            </rect>
          );
        })}
          </g>

          {ticks.map((tick) => (
            <line
              key={tick}
              x1={safeScale(xScale, tick)}
              x2={safeScale(xScale, tick)}
              y1={0}
              y2={contentHeight}
              stroke="#000"
              strokeDasharray="3,3"
              opacity={0.5}
              pointerEvents="none"
            />
          ))}

          {/* Y-axis: the stacked rows are concurrency levels, so label the row index
              the same repeated way as the task-count / blocking-reason charts. It
              lives inside the scrolling SVG, so the numbers track the rows as you
              scroll. */}
          <YAxisOverlay
            yScale={d3.scaleLinear().domain([0, Math.max(1, topRows)]).range([0, contentHeight])}
            width={width}
            // One label roughly every ~120px of the (tall, scrolling) chart, so some
            // are always in view rather than 50 rows apart and mostly off-screen.
            tickCount={Math.max(4, Math.round(contentHeight / 120))}
          />
        </svg>
      </div>
      {/* "More rows above/below" affordances: a fade plus a clickable chevron pill at
          each end, shown only while the gantt can scroll that way. The fades are
          click-through so they never block dragging the bars beneath them; each pill
          scrolls a page in its direction. They disappear at that end of the scroll
          (and entirely when every row already fits). */}
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

// formatCount renders an axis count compactly: 60000 -> "60k", 1500000 -> "1.5M".
function formatCount(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1e9) return `${+(n / 1e9).toFixed(1)}B`;
  if (abs >= 1e6) return `${+(n / 1e6).toFixed(1)}M`;
  if (abs >= 1e3) return `${+(n / 1e3).toFixed(1)}k`;
  return String(n);
}

// Roughly one repeated y-value label per this many pixels of chart width.
const Y_LABEL_SPACING = 450;

// YAxisOverlay draws horizontal value gridlines for a count chart, repeating the
// value label across the width — one column roughly every Y_LABEL_SPACING px, both
// edges included — and paints it ON TOP of the chart with a white halo so the value
// stays readable over the filled areas of a wide chart. Shared by the task-count and
// blocking-reason charts (tickCount left at the default) and the per-task gantt,
// which passes a larger tickCount so labels stay visible in its tall, scrolling SVG.
function YAxisOverlay({ yScale, width, tickCount = 4 }: { yScale: d3.ScaleLinear<number, number>; width: number; tickCount?: number }) {
  const left = 5;
  const right = Math.max(left + 1, width - 5);
  const intervals = Math.max(1, Math.round((right - left) / Y_LABEL_SPACING));
  const columns = Array.from({ length: intervals + 1 }, (_, i) => left + (i / intervals) * (right - left));
  // Skip the 0 baseline (it's implicit at the axis) and any non-integer ticks a
  // tiny range would produce.
  const ticks = yScale.ticks(tickCount).filter((tick) => Number.isInteger(tick) && tick > 0);
  return (
    <g pointerEvents="none">
      {ticks.map((tick) => {
        const y = safeScale(yScale, tick);
        const labelY = Math.max(9, y - 3);
        return (
          <g key={tick}>
            <line x1={left} x2={right} y1={y} y2={y} stroke="#94a3b8" strokeDasharray="3,3" opacity={0.45} />
            {columns.map((cx, i) => (
              <text
                key={i}
                x={cx}
                y={labelY}
                textAnchor={i === 0 ? "start" : i === columns.length - 1 ? "end" : "middle"}
                fontSize="10"
                fill="#475569"
                stroke="#ffffff"
                strokeWidth={2.5}
                paintOrder="stroke"
              >
                {formatCount(tick)}
              </text>
            ))}
          </g>
        );
      })}
    </g>
  );
}

// GapShading hatches the time ranges where no trace was collected, matching the
// component gantt's treatment so the overview charts read consistently. Drawn on
// top of the filled areas (it is faint) so a gap is visible even over a band.
function GapShading({
  gaps,
  xScale,
  height,
  patternId,
}: {
  gaps: Segment[];
  xScale: d3.ScaleLinear<number, number>;
  height: number;
  patternId: string;
}) {
  if (gaps.length === 0) return null;
  return (
    <>
      <defs>
        <pattern id={patternId} patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
          <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
          <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
        </pattern>
      </defs>
      {gaps.map((gap, index) => {
        const x = safeScale(xScale, gap.start_time);
        const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
        return <rect key={index} x={x} y={0} width={w} height={height} fill={`url(#${patternId})`} pointerEvents="none" />;
      })}
    </>
  );
}

// LoadingCurve is a placeholder silhouette shown while a chart's occupancy data
// is still loading (those queries take a while on a large scope). It is a
// deterministic mock density shape — not real data — drawn in muted gray with a
// bright highlight stripe that sweeps left→right (skeleton-shimmer style) so the
// panel clearly reads as "loading" rather than sitting blank. `id` must be
// unique per instance on the page (the clip path / gradient are referenced by id).
function LoadingCurve({
  width,
  height,
  id,
}: {
  width: number;
  height: number;
  id: string;
}) {
  const w = Math.max(1, width);
  const h = Math.max(1, height);
  const n = 96;
  // A per-instance phase derived from the id, so the two charts' mock curves
  // look different rather than identical twins. Deterministic (no Math.random)
  // so the shape stays stable across re-renders instead of reshuffling.
  let seed = 0;
  for (let i = 0; i < id.length; i++) seed += id.charCodeAt(i) * (i + 1);
  const ph = ((seed % 100) / 100) * Math.PI * 2;
  const pts: string[] = [];
  for (let i = 0; i <= n; i++) {
    const t = i / n;
    // An irregular, lopsided density profile: a few non-harmonic sine components
    // (so it never reads as periodic or mirror-symmetric) under a soft envelope
    // that lifts it off the baseline and brings it back down at both ends.
    const bumps =
      0.5 +
      0.22 * Math.sin(t * 6.0 + 0.6 + ph) +
      0.13 * Math.sin(t * 11.3 + 2.1 + ph * 1.7) +
      0.08 * Math.sin(t * 19.7 + 4.0 + ph * 0.6) +
      0.05 * Math.sin(t * 31.1 + 1.2 + ph * 2.3);
    const envelope = Math.pow(Math.sin(Math.PI * t), 0.35);
    const frac = Math.min(
      1,
      Math.max(0, 0.06 + 0.92 * Math.max(0, bumps) * envelope),
    );
    const x = 5 + t * (w - 10);
    const y = h - 4 - frac * (h - 12);
    pts.push(`${x.toFixed(1)},${y.toFixed(1)}`);
  }
  const d = `M${pts.join("L")}L${(w - 5).toFixed(1)},${h} L5,${h} Z`;
  const clipId = `lc-clip-${id}`;
  const gradId = `lc-grad-${id}`;
  return (
    <g pointerEvents="none">
      <defs>
        <clipPath id={clipId}>
          <path d={d} />
        </clipPath>
        {/* A wide, soft, low-contrast highlight band — translating the rect that
            carries it glides the band across the silhouette left→right, like the
            skeleton shimmer shown while images load on the web. The base gray
            stays steady; only this lighter band moves. */}
        <linearGradient id={gradId} x1="0" y1="0" x2="1" y2="0">
          <stop offset="0%" stopColor="#fff" stopOpacity="0" />
          <stop offset="25%" stopColor="#fff" stopOpacity="0" />
          <stop offset="50%" stopColor="#fff" stopOpacity="0.55" />
          <stop offset="75%" stopColor="#fff" stopOpacity="0" />
          <stop offset="100%" stopColor="#fff" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={d} fill="#cbd5e1" />
      <g clipPath={`url(#${clipId})`}>
        {/* The band enters just off the left edge and exits just off the right,
            so the loop restart lands off-screen and the sweep reads as one
            continuous, gently easing motion. */}
        <rect x={0} y={0} width={w} height={h} fill={`url(#${gradId})`}>
          <animate
            attributeName="x"
            from={-0.85 * w}
            to={0.85 * w}
            dur="1.8s"
            calcMode="spline"
            keyTimes="0;1"
            keySplines="0.4 0 0.6 1"
            repeatCount="indefinite"
          />
        </rect>
      </g>
    </g>
  );
}

// AggregatedTimeline is the level-of-detail replacement for ComponentTimeline
// when the visible range holds too many tasks to draw one element each. It draws
// a stacked-area density chart from the server's per-bin, per-"Kind-What" counts:
// a handful of <path>s instead of hundreds of thousands of <rect>s. Individual
// tasks are not hoverable/clickable here (zoom in for that), but hovering a band
// highlights it and the tasks of that kind at the cursor's time in the gantt.
function AggregatedTimeline({
  data,
  range,
  size,
  colorMap,
  highlightedKey,
  onHoverKey,
  segments,
  segmentsEnabled,
  showZoomHint,
}: {
  data: ComponentTimelineData | null;
  range: TimeRange;
  size: Size;
  colorMap: Record<string, string>;
  highlightedKey: string | null;
  // Hovering a band reports the kind + cursor time so the parent can light the
  // matching legend row, dim the other bands, and light that kind's tasks at the
  // cursor time in the gantt. Legend hover drives the same highlight in reverse.
  onHoverKey: (key: string | null, time: number | null) => void;
  segments: Segment[];
  segmentsEnabled: boolean;
  // When the per-task gantt is not shown, hint that zooming in reveals it.
  showZoomHint: boolean;
}) {
  const width = Math.max(1, size.width);
  const height = Math.max(1, size.height);
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];

  const gridlines = ticks.map((tick) => (
    <line key={tick} x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={height} stroke="#000" strokeDasharray="3,3" opacity={0.4} pointerEvents="none" />
  ));

  // Before the summary lands, still draw the time marks (and any uncollected-range
  // shading) so the region keeps its axis instead of collapsing to a "loading" box.
  if (!data) {
    return (
      <svg width={width} height={height} className="block">
        {gridlines}
        <GapShading gaps={gaps} xScale={xScale} height={height} patternId="count-gap-pattern" />
        <LoadingCurve width={width} height={height} id="count" />
        <text x={8} y={15} fontSize="11" fill="#94a3b8" pointerEvents="none">
          Task count · loading…
        </text>
      </svg>
    );
  }

  const { keys, bins } = data;
  const numBins = bins.length;
  const padTop = 22;
  const padBottom = 4;

  let maxTotal = 0;
  for (const row of bins) {
    let sum = 0;
    for (const v of row) sum += v;
    if (sum > maxTotal) maxTotal = sum;
  }
  const yScale = d3.scaleLinear().domain([0, Math.max(1, maxTotal)]).range([height - padBottom, padTop]);

  // Each bin covers an equal slice of the range the summary was computed over;
  // place it by its absolute center time so it lines up with the shared axis even
  // mid-pan (just like the per-task bars are placed by their absolute times).
  const binCenter = (i: number) => data.start_time + ((i + 0.5) / numBins) * (data.end_time - data.start_time);

  const areas = keys.map((key, ki) => {
    const tops: string[] = [];
    const bots: string[] = [];
    for (let i = 0; i < numBins; i++) {
      let base = 0;
      for (let k = 0; k < ki; k++) base += bins[i][k];
      const top = base + bins[i][ki];
      const x = safeScale(xScale, binCenter(i));
      tops.push(`${x},${yScale(top)}`);
      bots.push(`${x},${yScale(base)}`);
    }
    bots.reverse();
    return { key, d: `M${tops.join("L")}L${bots.join("L")}Z` };
  });

  const hasHighlight = highlightedKey !== null;

  return (
    <svg width={width} height={height} className="block">
      {areas.map(({ key, d }) => (
        <path
          key={key}
          d={d}
          fill={colorMap[key] ?? "#999999"}
          stroke="none"
          opacity={hasHighlight ? (highlightedKey === key ? 1 : 0.12) : 0.9}
          className="cursor-pointer"
          onMouseMove={(event) => {
            const svg = event.currentTarget.ownerSVGElement;
            if (!svg) {
              onHoverKey(key, null);
              return;
            }
            const rect = svg.getBoundingClientRect();
            onHoverKey(key, xScale.invert(event.clientX - rect.left));
          }}
          onMouseLeave={() => onHoverKey(null, null)}
        >
          <title>{key}</title>
        </path>
      ))}

      {gridlines}

      <GapShading gaps={gaps} xScale={xScale} height={height} patternId="count-gap-pattern" />

      <YAxisOverlay yScale={yScale} width={width} />

      <text x={8} y={15} fontSize="11" fill="#475569" pointerEvents="none" stroke="#ffffff" strokeWidth={2.5} paintOrder="stroke">
        Task count · {data.total.toLocaleString()} tasks{showZoomHint ? " · zoom in for individual tasks" : ""}
      </text>
    </svg>
  );
}

// ComponentMilestoneAreas is the bottom region: a stacked-area chart of blocking
// reasons over the same bins as the task-count chart above it. At each bin the
// stacked height shows how many in-flight tasks are blocked by each reason
// (milestone kind), colored to match the wavy lines. Drawn as one <path> per
// reason (not a rect per bin) so it stays cheap at the task chart's ~900 bins.
function ComponentMilestoneAreas({
  info,
  range,
  width,
  height,
  colorMap,
  highlightedKey,
  segments,
  segmentsEnabled,
  onHoverSegment,
  onHoverReason,
}: {
  info: StackedComponentInfo | null;
  range: TimeRange;
  width: number;
  height: number;
  colorMap: Record<string, string>;
  // The highlighted reason (from a legend hover or a band hover): its band stays
  // opaque while the rest dim.
  highlightedKey: string | null;
  segments: Segment[];
  segmentsEnabled: boolean;
  onHoverSegment: (segment: { kind: string; time: number } | null) => void;
  // Hovering a band also highlights it (and the matching legend reason); null clears.
  onHoverReason: (kind: string | null) => void;
}) {
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  const xAxisY = Math.max(0, height - 20);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];

  const data = info?.data ?? [];
  const kinds = info?.kinds ?? [];
  const loading = !info || data.length === 0;
  const maxTotal =
    d3.max(data, (point) => kinds.reduce((sum, kind) => sum + (point.values[kind] ?? 0), 0)) ?? 0;
  const yScale = d3.scaleLinear().domain([0, Math.max(1, maxTotal)]).range([Math.max(1, xAxisY - 4), 6]);

  // One stacked-area band per reason: trace the cumulative top edge left-to-right,
  // then the band's base edge right-to-left, and close. Each bin is placed by its
  // absolute center time so the bands line up with the shared axis during a pan.
  const areas = kinds.map((kind, ki) => {
    const tops: string[] = [];
    const bots: string[] = [];
    for (const point of data) {
      let base = 0;
      for (let k = 0; k < ki; k++) base += point.values[kinds[k]] ?? 0;
      const top = base + (point.values[kind] ?? 0);
      const x = safeScale(xScale, point.time);
      tops.push(`${x},${safeScale(yScale, top)}`);
      bots.push(`${x},${safeScale(yScale, base)}`);
    }
    bots.reverse();
    return { kind, d: tops.length ? `M${tops.join("L")}L${bots.join("L")}Z` : "" };
  });

  const hasHighlight = highlightedKey !== null;

  return (
    <svg width={width} height={height} className="block">
      {ticks.map((tick) => (
        <g key={tick} pointerEvents="none">
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={xAxisY} stroke="#000" strokeDasharray="3,3" opacity={0.3} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={xAxisY} y2={xAxisY + 5} stroke="#000" />
          <text x={safeScale(xScale, tick)} y={height - 4} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={xAxisY} y2={xAxisY} stroke="#000" pointerEvents="none" />

      {loading && <LoadingCurve width={width} height={xAxisY} id="reason" />}

      {areas.map(({ kind, d }) =>
        d ? (
          <path
            key={kind}
            d={d}
            fill={colorMap[kind] ?? "#9ca3af"}
            opacity={hasHighlight ? (highlightedKey === kind ? 1 : 0.12) : 0.9}
            className="cursor-pointer"
            // Report the reason + time under the cursor: highlight this band (and
            // the matching legend reason), and light the tasks blocked by that
            // reason at the cursor time in the view above.
            onMouseMove={(event) => {
              onHoverReason(kind);
              const svg = event.currentTarget.ownerSVGElement;
              if (!svg) return;
              const rect = svg.getBoundingClientRect();
              onHoverSegment({ kind, time: xScale.invert(event.clientX - rect.left) });
            }}
            onMouseLeave={() => {
              onHoverReason(null);
              onHoverSegment(null);
            }}
          >
            <title>{kind}</title>
          </path>
        ) : null,
      )}

      <GapShading gaps={gaps} xScale={xScale} height={xAxisY} patternId="blocking-gap-pattern" />

      <YAxisOverlay yScale={yScale} width={width} />
    </svg>
  );
}

function buildTopTaskRows(
  mainTask: Task,
  parentTask: Task | null,
  childTasks: Task[],
  xScale: d3.ScaleLinear<number, number>,
  height: number,
  milestoneBand: number,
) {
  const childLayout = childTasks.map((task) => ({ ...task, subTasks: [], level: 0 }) as LayoutTask);
  const maxYIndex = assignYIndices(childLayout);
  const barRegionHeight = height - TASK_VIEW_MARGIN_BOTTOM - TASK_VIEW_MARGIN_TOP;
  const nonSubTaskRegionHeight = TASK_VIEW_GROUP_GAP * 4 + TASK_VIEW_LARGE_TASK_HEIGHT * 2 + milestoneBand;
  const subTaskRegionHeight = Math.max(1, barRegionHeight - nonSubTaskRegionHeight);
  const childBarHeight = Math.min(TASK_VIEW_SUBTASK_BAR_HEIGHT, subTaskRegionHeight / Math.max(1, maxYIndex));
  const rows: TaskViewRow[] = [];
  const [rangeLeft, rangeRight] = xScale.range();

  const pushTask = (task: Task, y: number, rowHeight: number) => {
    // Clamp to the chart so the parent (which can span far beyond the focus
    // window) renders as a full-width context bar instead of off-screen coords.
    const x = Math.max(rangeLeft, safeScale(xScale, task.start_time));
    const right = Math.min(rangeRight, safeScale(xScale, task.end_time));
    rows.push({
      task,
      x,
      y,
      width: Math.max(1, right - x),
      height: rowHeight,
    });
  };

  if (parentTask) {
    pushTask(parentTask, TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP, TASK_VIEW_LARGE_TASK_HEIGHT);
  }

  pushTask(
    mainTask,
    TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2 + TASK_VIEW_LARGE_TASK_HEIGHT,
    TASK_VIEW_LARGE_TASK_HEIGHT,
  );

  const subTaskBaseY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 3 + TASK_VIEW_LARGE_TASK_HEIGHT * 2 + milestoneBand;
  for (const task of childLayout) {
    pushTask(task, subTaskBaseY + (task.yIndex ?? 0) * childBarHeight, childBarHeight * 0.75);
  }

  return rows;
}

function ComponentTaskView({
  mainTask,
  parentTask,
  childTasks,
  segments,
  segmentsEnabled,
  range,
  width,
  height,
  colorMap,
  colorMode,
  highlightedKey,
  highlightedTaskId,
  selectedTaskId,
  selectedMilestone,
  onHoverTask,
  onSelectTask,
  onOpenTask,
  onDeselect,
  onSelectMilestone,
}: {
  mainTask: Task | null;
  parentTask: Task | null;
  childTasks: Task[];
  segments: Segment[];
  segmentsEnabled: boolean;
  range: TimeRange;
  width: number;
  height: number;
  colorMap: Record<string, string>;
  colorMode: ColorMode;
  highlightedKey: string | null;
  highlightedTaskId: string | null;
  selectedTaskId: string | null;
  selectedMilestone: HoveredMilestone | null;
  onHoverTask: (task: Task | null) => void;
  onSelectTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onDeselect: () => void;
  onSelectMilestone: (milestone: HoveredMilestone | null) => void;
}) {
  if (!mainTask) {
    return <ComponentTopAxis width={width} height={height} range={range} />;
  }

  // Shares the time axis (range) with the component view's other charts. selecting
  // a task focuses this shared range on it (see selectTask), so the nested gantt
  // frames it; the parent, which can span far wider, is drawn clamped to the chart
  // (see buildTopTaskRows).
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  const milestoneBand = (mainTask.steps?.length ?? 0) > 0 ? TASK_VIEW_MILESTONE_BAND : 0;
  const rows = buildTopTaskRows(mainTask, parentTask, childTasks, xScale, height, milestoneBand);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];
  const divider1Y = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 1.5 + TASK_VIEW_LARGE_TASK_HEIGHT;
  const divider2Y = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2.5 + TASK_VIEW_LARGE_TASK_HEIGHT * 2 + milestoneBand;
  const parentLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP + 15;
  const currentLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2 + TASK_VIEW_LARGE_TASK_HEIGHT + 16;
  const subTasksLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 3 + TASK_VIEW_LARGE_TASK_HEIGHT * 2 + 16 + milestoneBand;

  return (
    <svg width={width} height={height} className="block" onClick={() => onDeselect()}>
      <defs>
        <pattern id="task-view-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
          <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
          <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
        </pattern>
      </defs>

      {gaps.map((gap, index) => {
        const x = safeScale(xScale, gap.start_time);
        const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
        return <rect key={index} x={x} y={TASK_VIEW_MARGIN_TOP} width={w} height={height - TASK_VIEW_MARGIN_BOTTOM} fill="url(#task-view-gap-pattern)" pointerEvents="none" />;
      })}

      {rows.map((row) => {
        const key = taskColorKey(row.task, colorMode);
        const taskHighlighted = highlightedTaskId === String(row.task.id);
        const keyHighlighted = highlightedKey === key;
        const hasHighlight = highlightedTaskId !== null || highlightedKey !== null;
        const highlighted = highlightedTaskId !== null ? taskHighlighted : highlightedKey !== null ? keyHighlighted : true;
        const selected = selectedTaskId != null && selectedTaskId === String(row.task.id);
        return (
          <rect
            key={String(row.task.id)}
            data-task-id={String(row.task.id)}
            x={row.x}
            y={row.y}
            width={row.width}
            height={Math.max(1, row.height)}
            fill={lookupColor(colorMap, row.task, colorMode)}
            stroke="#000000"
            strokeOpacity={selected || (hasHighlight && highlighted) ? 0.8 : 0.2}
            opacity={hasHighlight ? (highlighted ? 1 : 0.4) : selectedTaskId != null && !selected ? 0.6 : 1}
            className="cursor-pointer"
            onClick={(event) => {
              event.preventDefault();
              event.stopPropagation();
              onSelectTask(row.task);
            }}
            onDoubleClick={() => onOpenTask(row.task)}
          />
        );
      })}

      {ticks.map((tick) => (
        <g key={tick} pointerEvents="none">
          <text x={safeScale(xScale, tick)} y={11} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={16} y2={height} stroke="#000" strokeDasharray="3,3" opacity={0.5} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={16} y2={22} stroke="#000" />
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={16} y2={16} stroke="#000" pointerEvents="none" />

      <g className="daisen1-task-view-dividers" pointerEvents="none">
        <text x={5} y={parentLabelY} fontSize={20} textAnchor="start">
          Parent Task
        </text>
        <text x={5} y={currentLabelY} fontSize={20} textAnchor="start">
          Current Task
        </text>
        <text x={5} y={subTasksLabelY} fontSize={20} textAnchor="start">
          Subtasks
        </text>
        <line x1={0} x2={width} y1={divider1Y} y2={divider1Y} stroke="#000000" strokeDasharray="4" />
        <line x1={0} x2={width} y1={divider2Y} y2={divider2Y} stroke="#000000" strokeDasharray="4" />
      </g>

      {(() => {
        // Render each milestone as a curve over the interval it closes — from
        // the task start (or the previous milestone) up to the milestone — so
        // the curve shows how long, and on what reason, the task was blocked.
        const steps = milestonesOf(mainTask.steps).sort((a, b) => a.time - b.time);
        const barTop = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2 + TASK_VIEW_LARGE_TASK_HEIGHT;
        const centerY = barTop + TASK_VIEW_LARGE_TASK_HEIGHT + 6;
        return steps.map((step, index) => {
          const intervalStart = index === 0 ? mainTask.start_time : steps[index - 1].time;
          const x0 = safeScale(xScale, intervalStart);
          const x1 = safeScale(xScale, step.time);
          const color = colorMap[step.kind] ?? "#9ca3af";
          const d = wavyPath(x0, x1, centerY);
          // Affordance for the selected milestone: it stays full strength with a
          // thicker wave and a ringed dot, while the others dim.
          const selected = selectedMilestone != null && selectedMilestone.kind === step.kind && selectedMilestone.time === step.time;
          const dimmed = selectedMilestone != null && !selected;
          const opacity = dimmed ? 0.25 : 1;
          return (
            <g
              key={`milestone-${index}-${step.kind}`}
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                onSelectMilestone({
                  kind: step.kind,
                  what: step.what,
                  time: step.time,
                  blockedFor: step.time - intervalStart,
                });
              }}
            >
              {x1 - x0 >= 2 && (
                <>
                  {/* Invisible hit area so the thin wave is easy to click — no
                      fill or border, purely to capture the pointer. Carries the
                      milestone's details so the left column's pointer handler can
                      resolve the click (its capture eats this <g>'s onClick). */}
                  <rect
                    x={x0}
                    y={centerY - 8}
                    width={x1 - x0}
                    height={16}
                    fill="transparent"
                    pointerEvents="all"
                    data-ms-kind={step.kind}
                    data-ms-what={step.what}
                    data-ms-time={step.time}
                    data-ms-blocked={step.time - intervalStart}
                  />
                  <path d={d} fill="none" stroke={color} strokeWidth={selected ? 3 : 1.5} strokeLinecap="round" opacity={opacity} pointerEvents="none" />
                </>
              )}
              {selected && <circle cx={x1} cy={centerY} r={6} fill="none" stroke={color} strokeWidth={1.5} pointerEvents="none" />}
              <circle cx={x1} cy={centerY} r={selected ? 3.5 : 3} fill={color} stroke="#ffffff" strokeWidth={0.75} opacity={opacity} pointerEvents="none" />
            </g>
          );
        });
      })()}
    </svg>
  );
}

// A small uppercase section heading shared by the side-panel sections.
function SectionLabel({ children }: { children: string }) {
  return (
    <div className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{children}</div>
  );
}

// A blocking milestone the cursor is hovering, shown in the side panel in place
// of the selected task (so no SVG tooltip is needed on the chart).
interface HoveredMilestone {
  kind: string;
  what: string;
  time: number;
  blockedFor: number;
}

function SelectedTaskSection({
  task,
  milestone,
}: {
  task: Task | null;
  milestone: HoveredMilestone | null;
}) {
  // A hovered milestone takes over the panel and shows its blocking details.
  if (milestone) {
    const milestoneRows: [string, string][] = [
      ["Reason", milestone.kind],
      ["What", milestone.what || "-"],
      ["Released", smartString(milestone.time)],
      ["Blocked for", smartString(milestone.blockedFor)],
    ];
    return (
      <section>
        <SectionLabel>Selected milestone</SectionLabel>
        <div className="mt-2 rounded-lg border bg-muted/30 p-3">
          <div className="mb-2 break-all text-sm font-semibold">
            blocked on {milestone.kind}
          </div>
          <dl className="space-y-1.5 text-xs">
            {milestoneRows.map(([label, value]) => (
              <div key={label} className="grid grid-cols-[5.5rem_1fr] gap-x-3">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="break-all font-medium tabular-nums">{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      </section>
    );
  }

  // Nothing selected — just the inline prompt, no section heading.
  if (!task) return <TaskDetail task={null} />;
  // The selected task uses the shared TaskDetail (same as the task view), under a
  // section heading that mirrors the "Selected milestone" panel above.
  return (
    <section>
      <SectionLabel>Selected task</SectionLabel>
      <TaskDetail task={task} />
    </section>
  );
}

// ComponentLegend is the component view's binding of the shared Legend: the full
// interactive variant, with the color-mode toggle and hover highlighting wired in.
function ComponentLegend(props: {
  taskKeys: string[];
  colorMap: Record<string, string>;
  colorMode: ColorMode;
  onColorMode: (mode: ColorMode) => void;
  blockingReasons: string[];
  highlightedKey: string | null;
  onHighlight: (key: string | null) => void;
  highlightedReason: string | null;
  onHighlightReason: (kind: string | null) => void;
}) {
  return <Legend {...props} />;
}

function sanitizeRange(startTime: number, endTime: number): TimeRange {
  if (Number.isFinite(startTime) && Number.isFinite(endTime) && endTime > startTime) {
    return { startTime, endTime };
  }
  return { startTime: 0, endTime: 0.000001 };
}

// ComponentDetailView is the single-location view: a task timeline, task detail,
// and milestone bars, scoped to a location subtree. It renders for any node —
// a leaf (a real task row) shows just its tasks; an internal node (e.g. "ROB")
// aggregates every task beneath it but looks identical. The location tree powers
// the header breadcrumb (collapse up) and the drill-into control (descend).
// LocationSubtree renders the tree of locations beneath a scope, with every row
// clickable to jump into that location. Branches are collapsible (chevron),
// collapsed by default, so a deep scope stays compact instead of dumping its
// whole subtree at once.
function LocationSubtree({
  nodes,
  onNavigate,
  expanded,
  onToggle,
}: {
  nodes: LocationNode[];
  onNavigate: (path: string) => void;
  expanded: Set<string>;
  onToggle: (path: string) => void;
}) {
  return (
    <ul className="space-y-0.5">
      {nodes.map((node) => {
        const isBranch = node.children.length > 0;
        const open = isBranch && expanded.has(node.path);
        return (
          <li key={node.path}>
            <div className="flex items-center">
              {isBranch ? (
                <button
                  type="button"
                  className="flex h-6 w-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-primary"
                  onClick={() => onToggle(node.path)}
                  aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
                >
                  {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                </button>
              ) : (
                // A hollow dot marks a leaf (no children to expand).
                <span className="flex h-6 w-5 shrink-0 items-center justify-center" aria-hidden="true">
                  <span className="h-1.5 w-1.5 rounded-full border border-muted-foreground/50" />
                </span>
              )}
              <button
                type="button"
                className={cn(
                  "min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-xs transition-colors hover:bg-muted hover:text-primary",
                  isBranch ? "font-medium text-foreground" : "text-muted-foreground",
                )}
                onClick={() => onNavigate(node.path)}
                title={node.path}
              >
                {node.name}
              </button>
            </div>
            {/* Indent guide: a left border ties each child row back to its parent. */}
            {isBranch && open && (
              <div className="ml-2 border-l border-border pl-1.5">
                <LocationSubtree nodes={node.children} onNavigate={onNavigate} expanded={expanded} onToggle={onToggle} />
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function ComponentDetailView({ root }: { root: LocationNode }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const name = searchParams.get("name") ?? "";
  // `taskid` pins the nested hierarchy view (the task the page was opened on);
  // `sel` is the lighter-weight detail-panel selection a click sets. Splitting
  // them keeps a click from re-centering the hierarchy (see currentTask below) —
  // the same id/sel split the task view uses.
  const urlTaskId = searchParams.get("taskid");
  const urlSel = searchParams.get("sel");
  const { startTime: simStart, endTime: simEnd } = useSimulationRange();
  const urlStartTime = Number(searchParams.get("starttime") ?? simStart);
  const urlEndTime = Number(searchParams.get("endtime") ?? simEnd);
  const urlRange = sanitizeRange(urlStartTime, urlEndTime);
  const [viewRange, setViewRange] = useState<TimeRange>(urlRange);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(urlSel ?? urlTaskId);
  const [selectedTaskSeed, setSelectedTaskSeed] = useState<Task | null>(null);
  const dataRange = useDebouncedValue(viewRange, DATA_RANGE_DEBOUNCE_MS);
  const { ref, size } = useElementSize<HTMLDivElement>();
  const { data: segmentsData } = useSegments();
  // Resolve the selected task independently of the component-scoped query, so the
  // component in view can follow it when navigating to a parent task or subtask.
  const selectedTaskQuery = useMemo(() => (selectedTaskId ? { id: selectedTaskId } : {}), [selectedTaskId]);
  const { tasks: selectedTaskMatches, loading: selectedTaskLoading } = useTraceData(selectedTaskQuery);
  const selectedTaskFromFetch = selectedTaskMatches.find((task) => String(task.id) === selectedTaskId) ?? null;
  const selectedTaskFromSeed = selectedTaskSeed && String(selectedTaskSeed.id) === selectedTaskId ? selectedTaskSeed : null;
  const selectedTask = selectedTaskFromFetch ?? selectedTaskFromSeed;
  // The component page stays scoped to its own component. Selecting a task only
  // highlights it (in the nested hierarchy and the detail panel) — it never
  // re-scopes the page to the task's own component, which read as the view
  // "redirecting" on every click. Drilling into a task's component is what
  // double-click (open in the task view) is for.
  const selectedLocation = selectedTask?.location;
  const componentName = name;

  // Location hierarchy for the header: breadcrumb ancestors (collapse up) and the
  // current node's children (drill down). The view's data is scoped to this
  // location's subtree, so an internal node aggregates everything beneath it.
  const crumbs = useMemo(() => breadcrumbSegments(componentName), [componentName]);
  const scopeChildren = useMemo(() => findNode(root, componentName)?.children ?? [], [root, componentName]);
  const navigateToLocation = (path: string) => {
    const params = new URLSearchParams(searchParams);
    params.set("name", path);
    // Carry the *current* view range across the move. The zoom-to-URL sync uses
    // replaceState, which doesn't update React Router's searchParams, so reading
    // viewRange (not searchParams) is what keeps a zoomed-in range from snapping
    // back to the URL's stale/absent range on navigation.
    params.set("starttime", String(viewRange.startTime));
    params.set("endtime", String(viewRange.endTime));
    // Keep the current task selection when the destination scope still contains
    // it, so collapsing up to a parent location (or descending into the branch
    // that holds the task) doesn't lose the panel's selected task. The scope
    // aggregates a whole subtree, so the task is in view when the target is an
    // ancestor-or-equal of the current location (every breadcrumb "collapse up"),
    // or when the selected task's own location is at/under the target (drill-down
    // into its branch). Otherwise — a sibling branch that excludes the task — we
    // drop it so the panel never points at a task outside the view.
    const targetHoldsCurrent = componentName === path || componentName.startsWith(path + ".");
    const taskLoc = selectedLocation;
    const targetHoldsTask = !!taskLoc && (taskLoc === path || taskLoc.startsWith(path + "."));
    if (selectedTaskId && (targetHoldsCurrent || targetHoldsTask)) {
      params.set("taskid", selectedTaskId);
    } else {
      params.delete("taskid");
    }
    setSearchParams(params);
  };

  // Collapsible "locations under this scope" tree; branches start collapsed.
  const [expandedLocations, setExpandedLocations] = useState<Set<string>>(() => new Set());
  const toggleLocation = (path: string) =>
    setExpandedLocations((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });

  // Bin count, ~1 bin per 4px (a density chart gains nothing from sub-pixel bins,
  // and fewer bins make the heavy occupancy queries much cheaper), quantized to
  // 50 so a pixel-by-pixel resize does not refetch. Both the task-count and
  // blocking-reason charts use this count so their stacked areas line up.
  const numBins = Math.max(50, Math.min(300, Math.round((size.width - SIDE_COLUMN_WIDTH) / 4 / 50) * 50));
  const { info: stackedInfo, loading: infoLoading } = useStackedCompInfo(componentName, "ConcurrentTaskMilestones", dataRange.startTime, dataRange.endTime, numBins);

  // How tasks are grouped for coloring and for the task-count bands. The same
  // mode drives the server's grouping (below) and every taskColorKey here, so a
  // band's key always resolves to a color. Toggled from the legend.
  const [colorMode, setColorMode] = useState<ColorMode>("kind-what");

  // Level-of-detail: always fetch the cheap aggregated summary first. Its `total`
  // (tasks overlapping the range) decides whether the per-task view is affordable.
  const { data: agg, loading: aggLoading } = useComponentTimeline(componentName, dataRange.startTime, dataRange.endTime, numBins, colorMode);
  // Only fetch the raw tasks once the summary for THIS range confirms the count is
  // affordable. useComponentTimeline keeps the previous summary while a new range
  // loads, so we must also require the summary to cover the current range —
  // otherwise a stale, small total from a sparse range would green-light the huge
  // raw fetch for a freshly-selected dense range, defeating the level-of-detail
  // guard this whole path exists to provide. The echoed start/end round-trip
  // exactly, so the equality check is safe.
  // A summary describes the CURRENT range only once its echoed start/end match —
  // the hooks keep the previous range's data on screen while a new one loads, so
  // an unchecked summary would be answering the range we just zoomed away from.
  const aggMatchesRange = !!agg && agg.start_time === dataRange.startTime && agg.end_time === dataRange.endTime;
  const stackedMatchesRange =
    !!stackedInfo && stackedInfo.start_time === dataRange.startTime && stackedInfo.end_time === dataRange.endTime;
  // Only trust an EXACT total (sample === 1) for the raw/per-task decision. A
  // sampled estimate can undercount a dense scope and prematurely green-light a
  // full /api/trace fetch; the timeline hook runs an exact pass for any scope this
  // small, so a true sub-threshold scope resolves to sample === 1 here.
  const aggExact = aggMatchesRange && agg?.sample === 1;
  const rawEnabled = aggExact && (agg?.total ?? Infinity) <= RAW_TASK_THRESHOLD;
  const query = useMemo(
    () => (componentName && rawEnabled ? { scope: componentName, startTime: dataRange.startTime, endTime: dataRange.endTime } : {}),
    [dataRange.endTime, dataRange.startTime, componentName, rawEnabled],
  );
  const { tasks, loading: tasksLoading } = useTraceData(query);
  const selectedTaskFromComponent = useMemo(
    () => tasks.find((task) => String(task.id) === selectedTaskId) ?? null,
    [selectedTaskId, tasks],
  );
  // The detail panel follows the clicked task (the selection).
  const panelTask = selectedTask ?? selectedTaskFromComponent;
  // The nested hierarchy view (Parent / Current / Subtasks) is PINNED to the task
  // the page was opened on (the URL `taskid`) — a click does not re-center it, so
  // the hierarchy no longer jumps on every click. Double-click (open in the task
  // view) is how you move focus to another task.
  const inspectedTaskId = urlTaskId;
  const inspectedTaskQuery = useMemo(() => (inspectedTaskId ? { id: inspectedTaskId } : {}), [inspectedTaskId]);
  const { tasks: inspectedTaskMatches } = useTraceData(inspectedTaskQuery);
  const inspectedTaskFromFetch = inspectedTaskMatches.find((task) => String(task.id) === inspectedTaskId) ?? null;
  const inspectedTaskFromComponent = useMemo(
    () => tasks.find((task) => String(task.id) === inspectedTaskId) ?? null,
    [inspectedTaskId, tasks],
  );
  const currentTask = inspectedTaskFromFetch ?? inspectedTaskFromComponent;
  const parentTaskQuery = useMemo(
    () => (currentTask?.parent_id ? { id: String(currentTask.parent_id) } : {}),
    [currentTask?.parent_id],
  );
  const { tasks: parentTaskMatches, loading: parentTaskLoading } = useTraceData(parentTaskQuery);
  const parentTask = parentTaskMatches.find((task) => String(task.id) === String(currentTask?.parent_id)) ?? null;
  const childTaskQuery = useMemo(
    () => (currentTask ? { parentId: String(currentTask.id) } : {}),
    [currentTask?.id],
  );
  const { tasks: childTaskMatches, loading: childTasksLoading } = useTraceData(childTaskQuery);
  const childTasks = useMemo(
    () => (currentTask ? childTaskMatches.filter((task) => String(task.parent_id) === String(currentTask.id)) : []),
    [childTaskMatches, currentTask?.id],
  );
  const [hoveredTask, setHoveredTask] = useState<Task | null>(null);
  // A blocking milestone clicked on the current-task wavy line; shown in the side
  // panel (taking over the selected-task section) until a task is selected.
  const [selectedMilestone, setSelectedMilestone] = useState<HoveredMilestone | null>(null);
  const [highlightedKey, setHighlightedKey] = useState<string | null>(null);
  // Separate from highlightedKey (task "kind-what" keys): hovering a blocking
  // reason in the legend highlights its band without dimming the task charts,
  // whose keys live in a different namespace.
  const [highlightedReason, setHighlightedReason] = useState<string | null>(null);
  // What the blocking-reason chart and legend highlight: a hovered reason wins,
  // otherwise the selected blocking reason (the clicked milestone) stays lit so
  // selecting one keeps it highlighted.
  const reasonHighlight = highlightedReason ?? selectedMilestone?.kind ?? null;
  const dragRef = useRef<{ pointerId: number; x: number; range: TimeRange } | null>(null);
  const didDragRef = useRef(false);
  const pendingSelectTaskRef = useRef<Task | null>(null);
  const pendingMilestoneRef = useRef<HoveredMilestone | null>(null);
  // Click vs double-click timing for the left column. The pointer capture this
  // column takes for drag-panning swallows the nested view's native
  // click/dblclick, so select / make-current / deselect are resolved here instead.
  const pageLastClickRef = useRef<{ id: string; time: number } | null>(null);

  useEffect(() => {
    setViewRange(urlRange);
    setSelectedTaskId(urlSel ?? urlTaskId);
    setSelectedTaskSeed(null);
  }, [urlRange.startTime, urlRange.endTime, name, urlTaskId, urlSel]);

  useEffect(() => {
    if (!componentName) return;
    const params = new URLSearchParams(window.location.search);
    params.set("name", componentName);
    params.set("starttime", dataRange.startTime.toString());
    params.set("endtime", dataRange.endTime.toString());
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  }, [dataRange.endTime, dataRange.startTime, componentName]);

  // Commit the per-task gantt's visibility only when the summary describes the
  // current range. While a zoom's new summary loads we keep the previous decision,
  // so the gantt stays put instead of collapsing (and the task-count chart growing
  // to fill the gap) and then snapping back. We re-decide once the new summary lands.
  const [showGantt, setShowGantt] = useState(false);
  useEffect(() => {
    if (aggMatchesRange) setShowGantt(aggExact && (agg?.total ?? Infinity) <= RAW_TASK_THRESHOLD);
  }, [aggMatchesRange, aggExact, agg?.total]);

  // One global palette over every key that needs a color in this view — task
  // "kind-what" keys and blocking-reason kinds — so task bars and reasons are all
  // distinct and colored by the same mechanism. Assign it only once BOTH always-on
  // summaries for the current range are in, so the palette spans the whole key set
  // and never depends on which chart loaded first. Keep the previous assignment
  // through every transition: the first load shows gray until colors are ready (the
  // brief gap between the two summaries arriving), and a zoom never reshuffles.
  const colorMapRef = useRef<Record<string, string>>({});
  const colorMap = useMemo(() => {
    if (!aggMatchesRange || !stackedMatchesRange) return colorMapRef.current;
    const taskKeys = [...tasks, ...(currentTask ? [currentTask] : []), ...(parentTask ? [parentTask] : []), ...childTasks].map((t) => taskColorKey(t, colorMode));
    const reasonKeys = [...(stackedInfo?.kinds ?? []), ...milestonesOf(currentTask?.steps).map((step) => step.kind)];
    const next = buildColorMapFromKeys([...taskKeys, ...(agg?.keys ?? []), ...reasonKeys]);
    colorMapRef.current = next;
    return next;
  }, [aggMatchesRange, stackedMatchesRange, childTasks, currentTask, parentTask, tasks, stackedInfo, agg, colorMode]);

  // The task "kind-what" keys for the legend's Tasks subsection (distinct from
  // the blocking-reason kinds, so reasons no longer leak into the task legend).
  const taskColorKeys = useMemo(() => {
    // The always-on task-count view colors by the summary's kinds, so the legend
    // reflects those whenever the summary is loaded (still hover-to-highlight).
    if (agg) return agg.keys;
    const keys = new Set<string>();
    for (const task of [...tasks, ...(currentTask ? [currentTask] : []), ...(parentTask ? [parentTask] : []), ...childTasks]) {
      keys.add(taskColorKey(task, colorMode));
    }
    return Array.from(keys).sort();
  }, [childTasks, currentTask, parentTask, tasks, agg, colorMode]);

  // Default the color grouping to "kind" once the finer "kind-what" grouping
  // would produce more than 10 distinct pairs — too many to tell apart by color.
  // One-way and only until the user picks a mode, so it doesn't fight the toggle
  // or churn while zooming.
  const userPickedColorModeRef = useRef(false);
  const handleColorMode = (mode: ColorMode) => {
    userPickedColorModeRef.current = true;
    setColorMode(mode);
  };
  useEffect(() => {
    if (userPickedColorModeRef.current) return;
    if (colorMode === "kind-what" && taskColorKeys.length > 10) {
      setColorMode("kind");
    }
  }, [colorMode, taskColorKeys]);

  const selectableTaskById = useMemo(() => {
    const map = new Map<string, Task>();
    for (const task of [...tasks, ...(currentTask ? [currentTask] : []), ...(parentTask ? [parentTask] : []), ...childTasks]) {
      map.set(String(task.id), task);
    }
    return map;
  }, [childTasks, currentTask, parentTask, tasks]);
  const leftWidth = Math.max(1, size.width - SIDE_COLUMN_WIDTH - 1);
  // Up to four stacked regions: the selected-task view (optional — a thin axis when
  // no task is selected), the per-task gantt (optional — only when few enough tasks
  // are in range), the task-count density chart (always), and the blocking-reason
  // bars (always). Task view and blocking take fixed shares; the middle is split
  // between the gantt and the count, or given wholly to the count when no gantt.
  // Size the task view to its content — the parent/current rows plus one row per
  // subtask concurrency level — capped at the fixed ratio, so a task with only a
  // few subtasks doesn't leave a big empty band below them. Collapses to a thin
  // axis when no task is selected.
  const subtaskRowCount = useMemo(() => {
    if (!currentTask || childTasks.length === 0) return 0;
    const layout = childTasks.map((task) => ({ ...task, subTasks: [], level: 0 }) as LayoutTask);
    return assignYIndices(layout) + 1;
  }, [childTasks, currentTask]);
  const taskViewMilestoneBand = (currentTask?.steps?.length ?? 0) > 0 ? TASK_VIEW_MILESTONE_BAND : 0;
  const desiredTaskViewHeight =
    TASK_VIEW_MARGIN_TOP +
    TASK_VIEW_MARGIN_BOTTOM +
    TASK_VIEW_GROUP_GAP * 4 +
    TASK_VIEW_LARGE_TASK_HEIGHT * 2 +
    taskViewMilestoneBand +
    subtaskRowCount * TASK_VIEW_SUBTASK_BAR_HEIGHT;
  const taskViewHeight = currentTask
    ? Math.min(Math.round(size.height * TASK_VIEW_HEIGHT_RATIO), desiredTaskViewHeight)
    : TOP_AXIS_COMPACT_HEIGHT;
  const metricLineHeight = Math.round(size.height * COMPONENT_LINE_HEIGHT_RATIO);
  const middleHeight = Math.max(120, size.height - taskViewHeight - metricLineHeight);
  const countHeight = showGantt ? Math.min(220, Math.max(90, Math.round(middleHeight * 0.3))) : middleHeight;
  const ganttHeight = showGantt ? Math.max(80, middleHeight - countHeight) : 0;
  const dataPending = viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;

  // Count the debounced data-range update as in-flight render work so the off-screen
  // capture (which waits on the render-ready signal) does not snapshot an empty view
  // during the debounce window, before the real-range data is fetched.
  useRenderReady(dataPending);

  // Blocking reasons shown in this view, for the side-panel legend: the union of
  // the stacked bar chart's kinds (component-wide) and the current task's
  // milestones (the wavy lines), so the legend covers both.
  const blockingReasons = useMemo(() => {
    const set = new Set<string>(stackedInfo?.kinds ?? []);
    for (const step of milestonesOf(currentTask?.steps)) {
      set.add(step.kind);
    }
    return Array.from(set).sort();
  }, [stackedInfo, currentTask]);

  // What's hovered over the two stacked charts, with the time under the cursor:
  // a blocking-reason band (highlight the tasks blocked by that reason at that
  // moment) or a task-count band (highlight the tasks of that kind active at that
  // moment). Both scope the gantt highlight to the cursor's time rather than every
  // task of the kind — the timeline tasks carry milestones, so we recompute the
  // membership here rather than threading IDs through the chart data.
  const [hoveredSegment, setHoveredSegment] = useState<{ kind: string; time: number } | null>(null);
  const [hoveredCount, setHoveredCount] = useState<{ key: string; time: number } | null>(null);
  // A vertical guide line spanning the stacked charts, for reading off the same
  // time across them. Positioned via a ref (GPU transform) rather than state so a
  // mousemove doesn't re-render the whole chart stack on every pixel.
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
  const highlightedTaskIds = useMemo(() => {
    // Per-task highlighting from the stacked charts only applies when the per-task
    // gantt is shown.
    if (!rawEnabled) return null;
    const ids = new Set<string>();
    if (hoveredSegment) {
      for (const task of tasks) {
        if (task.start_time > hoveredSegment.time || task.end_time < hoveredSegment.time) continue;
        if (blockingKindAt(task.steps, hoveredSegment.time) === hoveredSegment.kind) ids.add(String(task.id));
      }
      return ids;
    }
    if (hoveredCount) {
      for (const task of tasks) {
        if (task.start_time > hoveredCount.time || task.end_time < hoveredCount.time) continue;
        if (taskColorKey(task, colorMode) === hoveredCount.key) ids.add(String(task.id));
      }
      return ids;
    }
    return null;
  }, [hoveredSegment, hoveredCount, tasks, rawEnabled, colorMode]);

  const shiftRange = (nextRange: TimeRange) => {
    if (!Number.isFinite(nextRange.startTime) || !Number.isFinite(nextRange.endTime)) return;
    if (nextRange.endTime <= nextRange.startTime) return;
    setViewRange(nextRange);
  };

  // Horizontal time-zoom (anchored under the cursor) plus horizontal pan, shared
  // by the parent wheel handler (task/metric regions) and the gantt's Ctrl/⌘+wheel.
  const zoomTimeRange = (deltaY: number, deltaX: number, pointerRatio: number) => {
    const duration = viewRange.endTime - viewRange.startTime;
    if (duration <= 0) return;

    let nextStartTime = viewRange.startTime;
    let nextEndTime = viewRange.endTime;

    if (deltaY !== 0) {
      const scale = Math.pow(1.001, deltaY);
      const pointerTime = viewRange.startTime + duration * pointerRatio;
      nextStartTime = pointerTime - (pointerTime - viewRange.startTime) * scale;
      nextEndTime = pointerTime + (viewRange.endTime - pointerTime) * scale;
    }

    if (deltaX !== 0) {
      const shift = (nextEndTime - nextStartTime) * deltaX * 0.001;
      nextStartTime += shift;
      nextEndTime += shift;
    }

    shiftRange({ startTime: nextStartTime, endTime: nextEndTime });
  };

  const handleWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    event.preventDefault();
    const bounds = event.currentTarget.getBoundingClientRect();
    const pointerX = Math.min(Math.max(event.clientX - bounds.left, 0), bounds.width);
    const pointerRatio = bounds.width > 0 ? pointerX / bounds.width : 0.5;
    zoomTimeRange(event.deltaY, event.deltaX, pointerRatio);
  };

  // The overview charts (task count, blocking reasons) are not zoom targets on a
  // plain scroll, but a modifier or trackpad pinch (Ctrl/⌘+scroll) should still
  // zoom the time axis — let only those through to the panel's wheel handler.
  const handleOverviewWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (!event.ctrlKey && !event.metaKey) event.stopPropagation();
  };

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { pointerId: event.pointerId, x: event.clientX, range: viewRange };
    didDragRef.current = false;
    pendingSelectTaskRef.current = null;
    pendingMilestoneRef.current = null;

    if (event.target instanceof Element) {
      const taskElement = event.target.closest("[data-task-id]");
      const taskId = taskElement?.getAttribute("data-task-id");
      if (taskId) {
        pendingSelectTaskRef.current = selectableTaskById.get(taskId) ?? null;
      } else {
        // A blocking-milestone hit area carries its details so the click can be
        // resolved here (the column's pointer capture eats the wave's own onClick).
        const ms = event.target.closest("[data-ms-kind]");
        if (ms) {
          pendingMilestoneRef.current = {
            kind: ms.getAttribute("data-ms-kind") ?? "",
            what: ms.getAttribute("data-ms-what") ?? "",
            time: Number(ms.getAttribute("data-ms-time")),
            blockedFor: Number(ms.getAttribute("data-ms-blocked")),
          };
        }
      }
    }
  };

  const handlePointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId || leftWidth <= 0) return;
    event.preventDefault();

    const pixelDelta = event.clientX - drag.x;
    if (Math.abs(pixelDelta) > 1) {
      didDragRef.current = true;
    }

    const duration = drag.range.endTime - drag.range.startTime;
    const timeDelta = (duration / leftWidth) * pixelDelta;
    shiftRange({
      startTime: drag.range.startTime - timeDelta,
      endTime: drag.range.endTime - timeDelta,
    });
  };

  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    dragRef.current = null;
    const pendingTask = pendingSelectTaskRef.current;
    const pendingMilestone = pendingMilestoneRef.current;
    pendingSelectTaskRef.current = null;
    pendingMilestoneRef.current = null;

    if (didDragRef.current) return;
    if (pendingMilestone) {
      selectMilestone(pendingMilestone);
      return;
    }
    if (!pendingTask) {
      // A click on empty space clears the selection.
      deselectTask();
      return;
    }

    // Single click selects; a second quick click on the same task makes it the
    // new current task (re-centering the nested hierarchy on it).
    const id = String(pendingTask.id);
    const now = Date.now();
    const last = pageLastClickRef.current;
    if (last && last.id === id && now - last.time < 350) {
      pageLastClickRef.current = null;
      makeTaskCurrent(pendingTask);
    } else {
      pageLastClickRef.current = { id, time: now };
      selectTask(pendingTask);
    }
  };

  const selectTask = (task: Task) => {
    if (didDragRef.current) {
      didDragRef.current = false;
      return;
    }
    const taskId = String(task.id);
    setSelectedTaskId(taskId);
    setSelectedTaskSeed(task);
    setSelectedMilestone(null);
    // A click only selects: it updates the detail panel and the URL `sel`. It does
    // NOT touch `taskid` (so the nested hierarchy view stays pinned to the opened
    // task) and does not move the time axis — nothing in the layout jumps.
    // Double-click opens the task view.
    const params = new URLSearchParams(window.location.search);
    params.set("sel", taskId);
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  };

  // Double-click a task (anywhere on the component page) to make it the current
  // task — stay on the component page and re-center the nested hierarchy on it
  // (set `taskid`), rather than leaving for the task view. Clearing `sel` lets the
  // selection default back to the new current task. The visible range is carried
  // across so the view does not snap.
  const makeTaskCurrent = (task: Task) => {
    const params = new URLSearchParams(searchParams);
    params.set("taskid", String(task.id));
    params.delete("sel");
    params.set("starttime", String(viewRange.startTime));
    params.set("endtime", String(viewRange.endTime));
    setSearchParams(params);
  };


  const deselectTask = () => {
    // Clear the selected task (the detail panel). Keep `taskid` so the nested
    // hierarchy view stays put, and use replaceState (not setSearchParams) so the
    // navigation-reset effect does not immediately re-select the current task.
    setSelectedTaskId(null);
    setSelectedTaskSeed(null);
    setSelectedMilestone(null);
    const params = new URLSearchParams(window.location.search);
    params.delete("sel");
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  };

  // Selecting a blocking reason (a milestone on the current task) takes over the
  // detail panel from the task and highlights that reason in the blocking-reason
  // chart and legend (see reasonHighlight). The task selection is cleared so the
  // panel shows the reason instead of the task.
  const selectMilestone = (milestone: HoveredMilestone) => {
    setSelectedMilestone(milestone);
    setSelectedTaskId(null);
    setSelectedTaskSeed(null);
    const params = new URLSearchParams(window.location.search);
    params.delete("sel");
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  };

  if (!name) {
    return (
      <div className="flex h-full items-center justify-center bg-white">
        <div className="space-y-3 rounded border border-slate-300 bg-slate-100 p-6">
          <div className="text-lg font-semibold">No component selected</div>
          <Button asChild>
            <Link to="/">Open Dashboard</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div ref={ref} className="daisen1-component-page">
      <div
        className="daisen1-component-left"
        style={{ width: leftWidth }}
        onWheel={handleWheel}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onMouseMove={moveCrosshair}
        onMouseLeave={hideCrosshair}
      >
        {/* Global horizontal (time) zoom — always available, independent of whether
            the per-task gantt is shown. Vertical/row zoom is gantt-specific and
            lives on the gantt's own toolbar instead. */}
        <TimeZoomControls onZoom={(dir) => zoomTimeRange(dir * 160, 0, 0.5)} className="absolute right-2 top-1" />
        {/* Three stacked regions. highlightedTaskId follows hover only (not the
            selected task), so selecting a task never dims the rest. Subtle
            border-t dividers separate the regions. */}
        <div
          className="daisen1-task-view relative"
          style={{ height: taskViewHeight }}
          // Plain scroll over the nested hierarchy does nothing (no zoom), but a
          // Ctrl/Cmd+scroll still zooms the time axis like the other regions —
          // handleOverviewWheel only swallows the unmodified wheel.
          onWheel={handleOverviewWheel}
        >
          <ComponentTaskView
            mainTask={currentTask}
            parentTask={parentTask}
            childTasks={childTasks}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            range={viewRange}
            width={leftWidth}
            height={taskViewHeight}
            colorMap={colorMap}
            colorMode={colorMode}
            highlightedKey={highlightedKey}
            highlightedTaskId={hoveredTask ? String(hoveredTask.id) : null}
            selectedTaskId={selectedTaskId}
            selectedMilestone={selectedMilestone}
            onHoverTask={setHoveredTask}
            onSelectTask={selectTask}
            onOpenTask={makeTaskCurrent}
            onDeselect={deselectTask}
            onSelectMilestone={setSelectedMilestone}
          />
          {/* Help opens only when a task is selected — that's when the hierarchy exists. */}
          {currentTask && (
            <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()}>
              <TaskHierarchyHelp className={CHART_HELP_BUTTON} />
            </div>
          )}
        </div>
        {/* Component tasks (per-task gantt) — optional: only when the range holds
            few enough tasks to draw each one. */}
        {showGantt && (
          <div className="daisen1-component-view relative border-t border-slate-200" style={{ height: ganttHeight }}>
            <ComponentTimeline
              name={componentName}
              tasks={tasks}
              segments={segmentsData?.segments ?? []}
              segmentsEnabled={segmentsData?.enabled ?? false}
              range={viewRange}
              size={{ width: leftWidth, height: ganttHeight }}
              colorMap={colorMap}
              colorMode={colorMode}
              highlightedKey={highlightedKey}
              highlightedTaskId={hoveredTask ? String(hoveredTask.id) : null}
              highlightedTaskIds={highlightedTaskIds}
              selectedTaskId={selectedTaskId}
              onHoverTask={setHoveredTask}
              onSelectTask={selectTask}
              onOpenTask={makeTaskCurrent}
              onDeselect={deselectTask}
              onZoom={zoomTimeRange}
              onRangeChange={shiftRange}
            />
            <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()}>
              <ComponentTaskViewHelp className={CHART_HELP_BUTTON} />
            </div>
          </div>
        )}
        {/* Task count (occupancy density by kind) — always shown. A plain scroll
            here does nothing; only a modifier/pinch (Ctrl/⌘+scroll) zooms time. */}
        <div
          className="daisen1-count-view relative border-t border-slate-200"
          style={{ height: countHeight }}
          onWheel={handleOverviewWheel}
        >
          {/* Rendered even while agg is null so the time marks stay put during load. */}
          <AggregatedTimeline
            data={agg}
            range={viewRange}
            size={{ width: leftWidth, height: countHeight }}
            colorMap={colorMap}
            highlightedKey={highlightedKey}
            onHoverKey={(key, time) => {
              // Light the matching legend row and dim the other bands, and scope the
              // gantt highlight to the tasks present at the cursor's time (not all of
              // the kind).
              setHighlightedKey(key);
              setHoveredCount(key !== null && time !== null ? { key, time } : null);
            }}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            showZoomHint={!showGantt}
          />
          <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()}>
            <TaskCountHelp className={CHART_HELP_BUTTON} />
          </div>
        </div>
        {/* Blocking reasons — always shown. Like the task-count chart: plain scroll
            is inert, a modifier/pinch (Ctrl/⌘+scroll) zooms time. */}
        <div
          className="daisen1-metric-view relative border-t border-slate-200"
          style={{ height: metricLineHeight }}
          onWheel={handleOverviewWheel}
        >
          <ComponentMilestoneAreas info={stackedInfo} range={viewRange} width={leftWidth} height={metricLineHeight} colorMap={colorMap} highlightedKey={reasonHighlight} segments={segmentsData?.segments ?? []} segmentsEnabled={segmentsData?.enabled ?? false} onHoverSegment={setHoveredSegment} onHoverReason={setHighlightedReason} />
          <div className={CHART_HELP_CORNER} onPointerDown={(e) => e.stopPropagation()}>
            <BlockingReasonsHelp className={CHART_HELP_BUTTON} />
          </div>
        </div>
        {/* Page-level navigation hint, bottom-left of the whole left column so it
            applies to every region (not just the gantt). The row-zoom tip only
            appears when the per-task gantt is shown, since that's the only region
            with rows. */}
        <div className="pointer-events-none absolute bottom-7 left-2 z-10 rounded bg-white/75 px-1.5 py-0.5 text-[10px] text-muted-foreground shadow-sm">
          Scroll/drag to pan · pinch or ⌘/Ctrl+scroll to zoom time{showGantt ? " · Alt+scroll for rows" : ""}
        </div>
        {/* Crosshair: a vertical guide at the cursor's x, spanning all the stacked
            charts so a feature can be read off at the same time across them. Solid
            (the gridlines are dashed) and click-through. */}
        <div
          ref={crosshairRef}
          aria-hidden="true"
          className="pointer-events-none absolute inset-y-0 left-0 z-10 w-px bg-slate-700/70 opacity-0"
          style={{ transform: "translateX(-1px)", willChange: "transform" }}
        />
      </div>

      <SidePanel className="flex select-none flex-col" style={{ width: SIDE_COLUMN_WIDTH }}>
        <div className="flex shrink-0 flex-col gap-2 border-b px-4 py-3">
          <div className="flex items-start justify-between gap-2">
            {/* Breadcrumb: ancestors are clickable (collapse up); the last is the
                current location. */}
            <nav className="flex min-w-0 flex-wrap items-center gap-x-1 gap-y-0.5 text-base font-bold leading-tight">
              {crumbs.map((crumb, index) => {
                const isLast = index === crumbs.length - 1;
                return (
                  <span key={crumb.path} className="flex min-w-0 items-center gap-1">
                    {index > 0 && <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
                    {isLast ? (
                      <span className="break-all">{crumb.label}</span>
                    ) : (
                      <button
                        type="button"
                        className="break-all font-normal text-muted-foreground hover:text-primary"
                        onClick={() => navigateToLocation(crumb.path)}
                      >
                        {crumb.label}
                      </button>
                    )}
                  </span>
                );
              })}
            </nav>
            <div className="flex shrink-0 items-center gap-2">
              {(dataPending || infoLoading || tasksLoading || aggLoading || selectedTaskLoading || parentTaskLoading || childTasksLoading) && (
                <span className="rounded border border-amber-300 bg-amber-50 px-1.5 py-0.5 text-[10px] font-medium text-amber-700">
                  Updating…
                </span>
              )}
              {selectedTaskId ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-7 gap-1 px-2 text-xs"
                  onClick={deselectTask}
                  title="Clear the selected task and return to the component overview"
                >
                  <X className="h-3.5 w-3.5" />
                  Deselect task
                </Button>
              ) : null}
            </div>
          </div>
          {/* For a scope, show the full expanded tree of locations beneath it; each
              row jumps into that location. Hidden for leaves. */}
          {scopeChildren.length > 0 && (
            <div className="flex flex-col gap-1 text-xs text-muted-foreground">
              <span>Locations under this scope</span>
              <div className="max-h-56 overflow-auto rounded-md border bg-muted/20 p-1.5">
                <LocationSubtree nodes={scopeChildren} onNavigate={navigateToLocation} expanded={expandedLocations} onToggle={toggleLocation} />
              </div>
            </div>
          )}
        </div>
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4">
          {/* The panel reflects the clicked/selected task (click-to-select),
              matching the task view; hovering only highlights the bar. */}
          <SelectedTaskSection
            task={panelTask}
            milestone={selectedMilestone}
          />
          <div className="-mx-4 border-t" />
          <ComponentLegend taskKeys={taskColorKeys} colorMap={colorMap} colorMode={colorMode} onColorMode={handleColorMode} blockingReasons={blockingReasons} highlightedKey={highlightedKey} onHighlight={setHighlightedKey} highlightedReason={reasonHighlight} onHighlightReason={setHighlightedReason} />
        </div>
      </SidePanel>
    </div>
  );
}

// ComponentPage renders the single detail view for any location. The name may be
// a leaf (e.g. "TLB.req_in") or an internal node (e.g. "TLB", which owns
// "TLB.req_in", "TLB.Top.incoming", …); the detail view scopes its data to the
// location subtree, so an internal node looks just like a leaf but aggregates
// every task beneath it. The hierarchy — used for the breadcrumb and the
// drill-into control — is derived from the flat /api/compnames list, split on ".".
export default function ComponentPage() {
  const { names, loading } = useComponentNames();
  const root = useMemo(() => buildLocationTree(names), [names]);

  if (loading && names.length === 0) {
    return <div className="flex h-full items-center justify-center text-muted-foreground">Loading…</div>;
  }

  return <ComponentDetailView root={root} />;
}
