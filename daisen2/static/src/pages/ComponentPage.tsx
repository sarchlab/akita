import * as d3 from "d3";
import { useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent, WheelEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { X } from "lucide-react";
import { Button } from "../components/ui/button";
import { SidePanel } from "../components/ui/side-panel";
import type { ComponentInfo } from "../hooks/useCompInfo";
import { useCompInfo } from "../hooks/useCompInfo";
import { useSegments } from "../hooks/useSegments";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useTraceData } from "../hooks/useTraceData";
import type { Segment, Task } from "../types/task";
import { buildColorMap, lookupColor, taskColorKey } from "../utils/taskColorCoder";
import { smartString } from "../utils/smartValue";
import { cn } from "../lib/utils";

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
const NUM_DOTS = 40;
const MIN_RANGE = 1e-12;
const TASK_VIEW_MARGIN_TOP = 20;
const TASK_VIEW_MARGIN_BOTTOM = 20;
const TASK_VIEW_GROUP_GAP = 10;
const TASK_VIEW_LARGE_TASK_HEIGHT = 15;

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

  return root;
}

function assignTaskLevel(task: LayoutTask, level: number) {
  task.level = level;
  for (const child of task.subTasks) {
    assignTaskLevel(child, level + 1);
  }
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

  return row.some((other) => {
    if (other.start_time <= task.start_time && other.end_time > task.start_time) return true;
    if (other.start_time < task.end_time && other.end_time >= task.end_time) return true;
    if (task.start_time <= other.start_time && task.end_time >= other.end_time) return true;
    if (task.start_time >= other.start_time && task.end_time <= other.end_time) return true;
    return false;
  });
}

function padTaskHeight(height: number) {
  return height > 10 ? height * 0.8 : height * 0.6;
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

  const taskHeight = parentLevelHeight / (globalMaxY + 1);
  const paddedTaskHeight = padTaskHeight(taskHeight) / 2;

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
        y: taskHeight * (child.yIndex ?? 0) + task.dim.y + (taskHeight - paddedTaskHeight) / 2,
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

function buildComponentTaskLayout(tasks: Task[], width: number, regionHeight: number, startTime: number, endTime: number) {
  const clonedTasks = cloneTasks(tasks);
  const root = buildTaskTree(clonedTasks);
  assignDimensions(root, {
    x: 0,
    y: 0,
    width,
    height: regionHeight,
    startTime,
    endTime,
  });

  return clonedTasks
    .sort((a, b) => a.level - b.level)
    .filter((task) => {
      if (task.level === 1) return true;
      if (!task.dim) return false;
      if (task.dim.width < 1) return false;
      if (task.dim.height < 1) return false;
      return true;
    });
}

function yDomain(info: ComponentInfo | null) {
  const max = d3.max(info?.data ?? [], (point) => point.value) ?? 0;
  return [0, max || 1] as [number, number];
}

function ComponentTopAxis({ width, height, range }: { width: number; height: number; range: TimeRange }) {
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);

  return (
    <svg width={width} height={height} className="block">
      {ticks.map((tick) => (
        <g key={tick}>
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={height} stroke="#000" strokeDasharray="3,3" opacity={0.5} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={5} y2={11} stroke="#000" />
          <text x={safeScale(xScale, tick)} y={18} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={5} y2={5} stroke="#000" />
    </svg>
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
  highlightedKey: string | null;
  highlightedTaskId: string | null;
  onHoverTask: (task: Task | null) => void;
  onSelectTask: (task: Task) => void;
}

function ComponentTimeline({
  name,
  tasks,
  segments,
  segmentsEnabled,
  range,
  size,
  colorMap,
  highlightedKey,
  highlightedTaskId,
  onHoverTask,
  onSelectTask,
}: ComponentTimelineProps) {
  const width = Math.max(1, size.width);
  const height = Math.max(1, size.height);
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  // The task bars fill the whole middle region; the metric line and the time-axis
  // labels live in the separate ComponentMetricLine region below.
  const taskLayout = buildComponentTaskLayout(tasks, width, height, range.startTime, range.endTime);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];

  return (
    <svg width={width} height={height} className="block">
      <defs>
        <pattern id="component-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
          <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
          <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
        </pattern>
      </defs>

      {gaps.map((gap, index) => {
        const x = safeScale(xScale, gap.start_time);
        const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
        return <rect key={index} x={x} y={0} width={w} height={height} fill="url(#component-gap-pattern)" pointerEvents="none" />;
      })}

      <g className="task-bar">
        {taskLayout.map((task) => {
          const dim = task.dim ?? {
            x: safeScale(xScale, task.start_time),
            y: 0,
            width: Math.max(1, safeScale(xScale, task.end_time) - safeScale(xScale, task.start_time)),
            height: 8,
          };
          const key = taskColorKey(task);
          const taskHighlighted = highlightedTaskId === String(task.id);
          const keyHighlighted = highlightedKey === key;
          const hasHighlight = highlightedTaskId !== null || highlightedKey !== null;
          const highlighted = highlightedTaskId !== null ? taskHighlighted : highlightedKey !== null ? keyHighlighted : true;
          return (
            <rect
              key={String(task.id)}
              data-task-id={String(task.id)}
              x={dim.x}
              y={dim.y}
              width={Math.max(1, dim.width)}
              height={Math.max(1, dim.height)}
              fill={lookupColor(colorMap, task)}
              stroke="#000000"
              strokeOpacity={hasHighlight && highlighted ? 0.8 : 0.2}
              opacity={highlighted ? 1 : 0.4}
              className="cursor-pointer"
              onMouseEnter={() => onHoverTask(task)}
              onMouseLeave={() => onHoverTask(null)}
              onClick={(event) => {
                event.preventDefault();
                onSelectTask(task);
              }}
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
          y2={height}
          stroke="#000"
          strokeDasharray="3,3"
          opacity={0.5}
          pointerEvents="none"
        />
      ))}
    </svg>
  );
}

// ComponentMetricLine is the bottom region: the component's concurrent-task
// metric drawn as a line chart, with its own value axis (left) and time axis
// (bottom). It is a fixed share of the window height (COMPONENT_LINE_HEIGHT_RATIO).
function ComponentMetricLine({
  info,
  range,
  width,
  height,
}: {
  info: ComponentInfo | null;
  range: TimeRange;
  width: number;
  height: number;
}) {
  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  const xAxisY = Math.max(0, height - 20);
  const yScale = d3.scaleLinear().domain(yDomain(info)).range([Math.max(1, xAxisY - 4), 6]);
  const linePath = d3
    .line<{ time: number; value: number }>()
    .x((point) => safeScale(xScale, point.time))
    .y((point) => safeScale(yScale, point.value))
    .curve(d3.curveCatmullRom.alpha(0.5))(info?.data ?? []);
  const yTicks = yScale.ticks(4);

  return (
    <svg width={width} height={height} className="block">
      {ticks.map((tick) => (
        <g key={tick} pointerEvents="none">
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={xAxisY} stroke="#000" strokeDasharray="3,3" opacity={0.5} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={xAxisY} y2={xAxisY + 5} stroke="#000" />
          <text x={safeScale(xScale, tick)} y={height - 4} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={xAxisY} y2={xAxisY} stroke="#000" pointerEvents="none" />

      <g transform="translate(40, 0)" pointerEvents="none">
        {yTicks.map((tick) => (
          <g key={tick}>
            <line x1={0} x2={width - 40} y1={safeScale(yScale, tick)} y2={safeScale(yScale, tick)} stroke="#ccc" strokeDasharray="3,3" opacity={0.5} />
            <text x={-8} y={safeScale(yScale, tick)} dy="0.32em" textAnchor="end" fontSize="10" fill="#4b5563">
              {d3.format(".1e")(tick)}
            </text>
          </g>
        ))}
      </g>

      {linePath ? <path d={linePath} fill="none" stroke="#2c7bb6" pointerEvents="none" /> : null}
    </svg>
  );
}

function buildTopTaskRows(
  mainTask: Task,
  parentTask: Task | null,
  childTasks: Task[],
  xScale: d3.ScaleLinear<number, number>,
  height: number,
) {
  const childLayout = childTasks.map((task) => ({ ...task, subTasks: [], level: 0 }) as LayoutTask);
  const maxYIndex = assignYIndices(childLayout);
  const barRegionHeight = height - TASK_VIEW_MARGIN_BOTTOM - TASK_VIEW_MARGIN_TOP;
  const nonSubTaskRegionHeight = TASK_VIEW_GROUP_GAP * 4 + TASK_VIEW_LARGE_TASK_HEIGHT * 2;
  const subTaskRegionHeight = Math.max(1, barRegionHeight - nonSubTaskRegionHeight);
  const childBarHeight = Math.min(10, subTaskRegionHeight / Math.max(1, maxYIndex));
  const rows: TaskViewRow[] = [];

  const pushTask = (task: Task, y: number, rowHeight: number) => {
    const x = safeScale(xScale, task.start_time);
    rows.push({
      task,
      x,
      y,
      width: Math.max(1, safeScale(xScale, task.end_time) - x),
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

  const subTaskBaseY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 3 + TASK_VIEW_LARGE_TASK_HEIGHT * 2;
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
  highlightedKey,
  highlightedTaskId,
  onHoverTask,
  onSelectTask,
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
  highlightedKey: string | null;
  highlightedTaskId: string | null;
  onHoverTask: (task: Task | null) => void;
  onSelectTask: (task: Task) => void;
}) {
  if (!mainTask) {
    return <ComponentTopAxis width={width} height={height} range={range} />;
  }

  const xScale = d3.scaleLinear().domain([range.startTime, range.endTime]).range([5, width - 5]);
  const ticks = xScale.ticks(12);
  const rows = buildTopTaskRows(mainTask, parentTask, childTasks, xScale, height);
  const gaps = segmentsEnabled ? gapSegments(segments, range.startTime, range.endTime) : [];
  const divider1Y = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 1.5 + TASK_VIEW_LARGE_TASK_HEIGHT;
  const divider2Y = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2.5 + TASK_VIEW_LARGE_TASK_HEIGHT * 2;
  const parentLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP + 15;
  const currentLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2 + TASK_VIEW_LARGE_TASK_HEIGHT + 16;
  const subTasksLabelY = TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 3 + TASK_VIEW_LARGE_TASK_HEIGHT * 2 + 16;

  return (
    <svg width={width} height={height} className="block">
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
        const key = taskColorKey(row.task);
        const taskHighlighted = highlightedTaskId === String(row.task.id);
        const keyHighlighted = highlightedKey === key;
        const hasHighlight = highlightedTaskId !== null || highlightedKey !== null;
        const highlighted = highlightedTaskId !== null ? taskHighlighted : highlightedKey !== null ? keyHighlighted : true;
        return (
          <rect
            key={String(row.task.id)}
            data-task-id={String(row.task.id)}
            x={row.x}
            y={row.y}
            width={row.width}
            height={Math.max(1, row.height)}
            fill={lookupColor(colorMap, row.task)}
            stroke="#000000"
            strokeOpacity={hasHighlight && highlighted ? 0.8 : 0.2}
            opacity={highlighted ? 1 : 0.4}
            className="cursor-pointer"
            onMouseEnter={() => onHoverTask(row.task)}
            onMouseLeave={() => onHoverTask(null)}
            onClick={(event) => {
              event.preventDefault();
              onSelectTask(row.task);
            }}
          >
            <title>
              {row.task.kind} - {row.task.what}
              {"\n"}
              {smartString(row.task.start_time)} to {smartString(row.task.end_time)}
            </title>
          </rect>
        );
      })}

      {ticks.map((tick) => (
        <g key={tick} pointerEvents="none">
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={height} stroke="#000" strokeDasharray="3,3" opacity={0.5} />
          <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={5} y2={11} stroke="#000" />
          <text x={safeScale(xScale, tick)} y={18} textAnchor="middle" fontSize="12" fill="#000">
            {formatAxisTick(tick)}
          </text>
        </g>
      ))}
      <line x1={5} x2={width - 5} y1={5} y2={5} stroke="#000" pointerEvents="none" />

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

      {mainTask.steps?.map((step, index) => (
        <circle
          key={`${step.kind}-${step.what}-${index}`}
          cx={safeScale(xScale, step.time)}
          cy={TASK_VIEW_MARGIN_TOP + TASK_VIEW_GROUP_GAP * 2 + TASK_VIEW_LARGE_TASK_HEIGHT + TASK_VIEW_LARGE_TASK_HEIGHT / 2}
          r={3}
          fill="#ff0000"
          stroke="#ffffff"
          pointerEvents="none"
        >
          <title>
            {step.kind}: {step.what} at {smartString(step.time)}
          </title>
        </circle>
      ))}
    </svg>
  );
}

// A small uppercase section heading shared by the side-panel sections.
function SectionLabel({ children }: { children: string }) {
  return (
    <div className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{children}</div>
  );
}

function SelectedTaskSection({ task }: { task: Task | null }) {
  if (!task) {
    return (
      <section>
        <SectionLabel>Selected task</SectionLabel>
        <p className="mt-2 text-xs text-muted-foreground">
          Hover or click a task in the chart to see its details.
        </p>
      </section>
    );
  }

  const rows: [string, string][] = [
    ["ID", String(task.id)],
    ["Kind", task.kind],
    ["What", task.what],
    ["Where", task.location || "-"],
    ["Start", smartString(task.start_time)],
    ["End", smartString(task.end_time)],
    ["Duration", smartString(task.end_time - task.start_time)],
  ];

  return (
    <section>
      <SectionLabel>Selected task</SectionLabel>
      <div className="mt-2 rounded-lg border bg-muted/30 p-3">
        <div className="mb-2 break-all text-sm font-semibold">
          {task.kind} · {task.what}
        </div>
        <dl className="space-y-1.5 text-xs">
          {rows.map(([label, value]) => (
            <div key={label} className="grid grid-cols-[4.5rem_1fr] gap-x-3">
              <dt className="text-muted-foreground">{label}</dt>
              <dd className="break-all font-medium tabular-nums">{value}</dd>
            </div>
          ))}
        </dl>
      </div>
    </section>
  );
}

function ComponentLegend({
  colorMap,
  highlightedKey,
  onHighlight,
}: {
  colorMap: Record<string, string>;
  highlightedKey: string | null;
  onHighlight: (key: string | null) => void;
}) {
  const entries = Object.entries(colorMap);
  if (entries.length === 0) return null;

  return (
    <section>
      <SectionLabel>Legend</SectionLabel>
      <ul className="mt-2 space-y-0.5">
        {entries.map(([key, color]) => {
          const dimmed = highlightedKey !== null && highlightedKey !== key;
          return (
            <li key={key}>
              <button
                type="button"
                className={cn(
                  "flex w-full items-center gap-2 rounded px-1.5 py-1 text-left text-xs transition-colors hover:bg-muted",
                  dimmed && "opacity-40",
                )}
                onMouseEnter={() => onHighlight(key)}
                onMouseLeave={() => onHighlight(null)}
                onFocus={() => onHighlight(key)}
                onBlur={() => onHighlight(null)}
              >
                <span
                  className="h-3 w-5 shrink-0 rounded-sm border border-black/30"
                  style={{ backgroundColor: color }}
                />
                <span className="truncate">{key}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function sanitizeRange(startTime: number, endTime: number): TimeRange {
  if (Number.isFinite(startTime) && Number.isFinite(endTime) && endTime > startTime) {
    return { startTime, endTime };
  }
  return { startTime: 0, endTime: 0.000001 };
}

export default function ComponentPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const name = searchParams.get("name") ?? "";
  const urlTaskId = searchParams.get("taskid");
  const { startTime: simStart, endTime: simEnd } = useSimulationRange();
  const urlStartTime = Number(searchParams.get("starttime") ?? simStart);
  const urlEndTime = Number(searchParams.get("endtime") ?? simEnd);
  const urlRange = sanitizeRange(urlStartTime, urlEndTime);
  const [viewRange, setViewRange] = useState<TimeRange>(urlRange);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(urlTaskId);
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
  // The component in view tracks the selected task's location; clicking a parent
  // task or subtask navigates to that task's component (issue #156).
  const componentName = selectedTask?.location || name;

  const { info, loading: infoLoading } = useCompInfo(componentName, "ConcurrentTask", dataRange.startTime, dataRange.endTime, NUM_DOTS);
  const query = useMemo(
    () => (componentName ? { where: componentName, startTime: dataRange.startTime, endTime: dataRange.endTime } : {}),
    [dataRange.endTime, dataRange.startTime, componentName],
  );
  const { tasks, loading: tasksLoading } = useTraceData(query);
  const selectedTaskFromComponent = useMemo(
    () => tasks.find((task) => String(task.id) === selectedTaskId) ?? null,
    [selectedTaskId, tasks],
  );
  const currentTask = selectedTask ?? selectedTaskFromComponent;
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
  const [highlightedKey, setHighlightedKey] = useState<string | null>(null);
  const dragRef = useRef<{ pointerId: number; x: number; range: TimeRange } | null>(null);
  const didDragRef = useRef(false);
  const pendingSelectTaskRef = useRef<Task | null>(null);

  useEffect(() => {
    setViewRange(urlRange);
    setSelectedTaskId(urlTaskId);
    setSelectedTaskSeed(null);
  }, [urlRange.startTime, urlRange.endTime, name, urlTaskId]);

  useEffect(() => {
    if (!componentName) return;
    const params = new URLSearchParams(window.location.search);
    params.set("name", componentName);
    params.set("starttime", dataRange.startTime.toString());
    params.set("endtime", dataRange.endTime.toString());
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  }, [dataRange.endTime, dataRange.startTime, componentName]);

  const colorMap = useMemo(
    () => buildColorMap([...tasks, ...(currentTask ? [currentTask] : []), ...(parentTask ? [parentTask] : []), ...childTasks]),
    [childTasks, currentTask, parentTask, tasks],
  );
  const selectableTaskById = useMemo(() => {
    const map = new Map<string, Task>();
    for (const task of [...tasks, ...(currentTask ? [currentTask] : []), ...(parentTask ? [parentTask] : []), ...childTasks]) {
      map.set(String(task.id), task);
    }
    return map;
  }, [childTasks, currentTask, parentTask, tasks]);
  const leftWidth = Math.max(1, size.width - SIDE_COLUMN_WIDTH - 1);
  // Three stacked regions sized as shares of the window: task view (20%, or a thin
  // axis in component mode), metric line (20%), and the timeline filling the rest.
  const taskViewHeight = currentTask ? Math.round(size.height * TASK_VIEW_HEIGHT_RATIO) : TOP_AXIS_COMPACT_HEIGHT;
  const metricLineHeight = Math.round(size.height * COMPONENT_LINE_HEIGHT_RATIO);
  const timelineHeight = Math.max(60, size.height - taskViewHeight - metricLineHeight);
  const dataPending = viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;

  const shiftRange = (nextRange: TimeRange) => {
    if (!Number.isFinite(nextRange.startTime) || !Number.isFinite(nextRange.endTime)) return;
    if (nextRange.endTime <= nextRange.startTime) return;
    setViewRange(nextRange);
  };

  const handleWheel = (event: WheelEvent<HTMLDivElement>) => {
    event.preventDefault();
    const duration = viewRange.endTime - viewRange.startTime;
    if (duration <= 0) return;

    let nextStartTime = viewRange.startTime;
    let nextEndTime = viewRange.endTime;

    if (event.deltaY !== 0) {
      const bounds = event.currentTarget.getBoundingClientRect();
      const pointerX = Math.min(Math.max(event.clientX - bounds.left, 0), bounds.width);
      const pointerRatio = bounds.width > 0 ? pointerX / bounds.width : 0.5;
      const scale = Math.pow(1.001, event.deltaY);
      const pointerTime = viewRange.startTime + duration * pointerRatio;
      nextStartTime = pointerTime - (pointerTime - viewRange.startTime) * scale;
      nextEndTime = pointerTime + (viewRange.endTime - pointerTime) * scale;
    }

    if (event.deltaX !== 0) {
      const shift = (nextEndTime - nextStartTime) * event.deltaX * 0.001;
      nextStartTime += shift;
      nextEndTime += shift;
    }

    shiftRange({ startTime: nextStartTime, endTime: nextEndTime });
  };

  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { pointerId: event.pointerId, x: event.clientX, range: viewRange };
    didDragRef.current = false;
    pendingSelectTaskRef.current = null;

    if (event.target instanceof Element) {
      const taskElement = event.target.closest("[data-task-id]");
      const taskId = taskElement?.getAttribute("data-task-id");
      pendingSelectTaskRef.current = taskId ? (selectableTaskById.get(taskId) ?? null) : null;
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
    pendingSelectTaskRef.current = null;

    if (pendingTask && !didDragRef.current) {
      selectTask(pendingTask);
    }
  };

  const focusRangeForTask = (task: Task) => {
    const duration = task.end_time - task.start_time;
    const fallbackPadding = Math.max(MIN_RANGE, (viewRange.endTime - viewRange.startTime) * 0.05);
    const padding = duration > 0 ? duration * 0.2 : fallbackPadding;
    return sanitizeRange(task.start_time - padding, task.end_time + padding);
  };

  const selectTask = (task: Task) => {
    if (didDragRef.current) {
      didDragRef.current = false;
      return;
    }
    const taskId = String(task.id);
    setSelectedTaskId(taskId);
    setSelectedTaskSeed(task);
    setViewRange(focusRangeForTask(task));

    const params = new URLSearchParams(window.location.search);
    params.set("name", task.location || name);
    params.set("taskid", taskId);
    window.history.replaceState(null, "", `/component?${params.toString()}`);
  };

  const deselectTask = () => {
    // Clear the selected task and collapse the task panel back to the overview.
    // Goes through react-router (not the raw replaceState that selectTask uses)
    // so `name`/`searchParams` are re-synced: keep the component currently in
    // view (componentName — which may differ from the URL's original `name`
    // after walking to a parent/subtask in another component) and the current
    // zoom range, just without a selected task.
    setSelectedTaskId(null);
    setSelectedTaskSeed(null);
    const params = new URLSearchParams();
    params.set("name", componentName);
    params.set("starttime", String(viewRange.startTime));
    params.set("endtime", String(viewRange.endTime));
    setSearchParams(params, { replace: true });
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
      >
        {/* Three stacked regions. highlightedTaskId follows hover only (not the
            selected task), so selecting a task never dims the rest. Subtle
            border-t dividers separate the regions. */}
        <div className="daisen1-task-view" style={{ height: taskViewHeight }}>
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
            highlightedKey={highlightedKey}
            highlightedTaskId={hoveredTask ? String(hoveredTask.id) : null}
            onHoverTask={setHoveredTask}
            onSelectTask={selectTask}
          />
        </div>
        <div className="daisen1-component-view border-t border-slate-200" style={{ height: timelineHeight }}>
          <ComponentTimeline
            name={componentName}
            tasks={tasks}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            range={viewRange}
            size={{ width: leftWidth, height: timelineHeight }}
            colorMap={colorMap}
            highlightedKey={highlightedKey}
            highlightedTaskId={hoveredTask ? String(hoveredTask.id) : null}
            onHoverTask={setHoveredTask}
            onSelectTask={selectTask}
          />
        </div>
        <div className="daisen1-metric-view border-t border-slate-200" style={{ height: metricLineHeight }}>
          <ComponentMetricLine info={info} range={viewRange} width={leftWidth} height={metricLineHeight} />
        </div>
      </div>

      <SidePanel className="flex select-none flex-col" style={{ width: SIDE_COLUMN_WIDTH }}>
        <div className="flex shrink-0 items-center justify-between gap-2 border-b px-4 py-3">
          <h2 className="min-w-0 break-all text-lg font-bold leading-tight">{componentName}</h2>
          <div className="flex shrink-0 items-center gap-2">
            {(dataPending || infoLoading || tasksLoading || selectedTaskLoading || parentTaskLoading || childTasksLoading) && (
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
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4">
          <SelectedTaskSection task={hoveredTask} />
          <ComponentLegend colorMap={colorMap} highlightedKey={highlightedKey} onHighlight={setHighlightedKey} />
        </div>
      </SidePanel>
    </div>
  );
}
