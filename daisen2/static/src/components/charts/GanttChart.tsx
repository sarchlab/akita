import * as d3 from "d3";
import type { Segment, Task } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../../utils/taskColorCoder";
import type { ColorMode } from "../../utils/taskColorCoder";
import { milestonesOf, wavyPath } from "../../utils/milestoneViz";
import { smartString } from "../../utils/smartValue";

interface GanttChartProps {
  // Ancestors root-first: [root, …, immediate parent], stacked above the current
  // task. The current task's subtree descends in `levels`.
  ancestors?: Task[];
  mainTask?: Task | null;
  // Descendant levels: [children, grandchildren, …]; each is drawn as its own
  // concurrency-packed band below the current task.
  levels?: Task[][];
  segments?: Segment[];
  segmentsEnabled?: boolean;
  // Color map shared with the page's legend so bars and legend swatches match.
  colorMap?: Record<string, string>;
  // Whether tasks are colored by kind alone or the finer kind-what pair; must
  // match how `colorMap` was built so swatch and bar colors agree.
  colorMode?: ColorMode;
  // Controlled selection: the parent owns which task is highlighted.
  selectedId?: string | number | null;
  onSelectTask?: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
  // Clicking the chart background (anywhere not on a bar) clears the selection.
  onDeselect?: () => void;
  // Show a button to load the next descendant level (a deeper subtree exists and
  // was not auto-loaded). expanding disables it while a level loads.
  canExpand?: boolean;
  expanding?: boolean;
  onExpandNext?: () => void;
}

const MARGIN = { top: 28, right: 12, bottom: 28, left: 8 };
const ROW_HEIGHT = 14;
const HEADER_ROW_HEIGHT = 22;
// Vertical room reserved below the current task bar for the blocking-reason
// curves (only when the task has milestones).
const MILESTONE_BAND = 22;
const LABEL_H = 18;
const HEADER_BAR_H = 18;
const DESC_BAR_H = 9;
const WIDTH = 1200;

function safeScale(scale: d3.ScaleLinear<number, number>, value: number) {
  return scale(value) ?? 0;
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

export default function GanttChart({
  ancestors = [],
  mainTask = null,
  levels = [],
  segments = [],
  segmentsEnabled = false,
  colorMap: colorMapProp,
  colorMode = "kind-what",
  selectedId = null,
  onSelectTask,
  onOpenTask,
  onDeselect,
  canExpand = false,
  expanding = false,
  onExpandNext,
}: GanttChartProps) {
  const allTasks = [...ancestors, ...(mainTask ? [mainTask] : []), ...levels.flat()];

  const milestoneSteps = milestonesOf(mainTask?.steps).sort((a, b) => a.time - b.time);
  const milestoneBand = milestoneSteps.length ? MILESTONE_BAND : 0;

  // Lane-assign each descendant level independently (clones, so props aren't
  // mutated). Each level is a set of siblings, so plain concurrency packing fits.
  const levelLayouts = levels.map((level) => {
    const tasks = level.map((task) => ({ ...task }));
    assignYIndices(tasks);
    const lanes = Math.max(1, Math.max(0, ...tasks.map((task) => task.yIndex ?? 0)) + 1);
    return { tasks, lanes };
  });

  if (allTasks.length === 0) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No tasks to display.</div>;
  }

  // Scale time to the current task and its descendants (the focus). Ancestors run
  // far wider (the root spans the whole trace), so including them would squash the
  // focus into a sliver; instead they are clamped to the chart as context bars.
  const focusTasks = mainTask ? [mainTask, ...levels.flat()] : allTasks;
  const timeStart = Math.min(...focusTasks.map((task) => task.start_time));
  const timeEnd = Math.max(...focusTasks.map((task) => task.end_time));
  const padding = (timeEnd - timeStart) * 0.02 || 1e-12;
  const startTime = timeStart - padding;
  const endTime = timeEnd + padding;
  const innerWidth = WIDTH - MARGIN.left - MARGIN.right;
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([MARGIN.left, WIDTH - MARGIN.right]);
  const colorMap =
    colorMapProp ??
    buildColorMapFromKeys([...allTasks.map((task) => taskColorKey(task, colorMode)), ...milestoneSteps.map((step) => step.kind)]);

  // Vertical layout, top → bottom: ancestor rows (root first), current task (with
  // its milestone band), then one labeled band per descendant level.
  let cursor = MARGIN.top + 8;
  const ancestorRows = ancestors.map((task) => {
    const top = cursor;
    cursor += HEADER_ROW_HEIGHT;
    return { task, top };
  });
  const currentTop = mainTask ? cursor : null;
  if (mainTask) cursor += HEADER_ROW_HEIGHT + milestoneBand;
  const levelRows = levelLayouts.map(({ tasks, lanes }, index) => {
    const labelTop = cursor;
    cursor += LABEL_H;
    const tasksTop = cursor;
    cursor += lanes * ROW_HEIGHT + 6;
    return { index, tasks, labelTop, tasksTop };
  });
  const height = cursor + MARGIN.bottom + 8;

  const xTicks = xScale.ticks(12);
  const gaps = segmentsEnabled ? gapSegments(segments, startTime, endTime) : [];

  // Blocking-reason curves for the current task: each milestone closes an interval
  // [previous milestone (or task start) → milestone], drawn as a wavy arc colored
  // by the released reason with a node at the release point.
  const milestoneCenterY = currentTop != null ? currentTop + HEADER_BAR_H + 6 : 0;
  const milestoneMarks =
    mainTask && currentTop != null
      ? milestoneSteps.map((step, index) => {
          const intervalStart = index === 0 ? mainTask.start_time : milestoneSteps[index - 1].time;
          const x0 = safeScale(xScale, intervalStart);
          const x1 = safeScale(xScale, step.time);
          const color = colorMap[step.kind] ?? "#9ca3af";
          const blockedFor = step.time - intervalStart;
          const d = wavyPath(x0, x1, milestoneCenterY);
          const tip = `blocked on ${step.kind} (${step.what}) for ${smartString(blockedFor)}`;
          return (
            <g key={`milestone-${index}-${step.kind}`}>
              {x1 - x0 >= 2 && (
                <>
                  <path d={d} fill="none" stroke={color} strokeWidth={1.5} strokeLinecap="round" pointerEvents="none" />
                  <path d={d} fill="none" stroke="transparent" strokeWidth={12} pointerEvents="stroke">
                    <title>{tip}</title>
                  </path>
                </>
              )}
              <circle cx={x1} cy={milestoneCenterY} r={3} fill={color} stroke="#ffffff" strokeWidth={0.75}>
                <title>{`${tip} — released at ${smartString(step.time)}`}</title>
              </circle>
            </g>
          );
        })
      : null;

  const renderBar = (task: Task, top: number, barHeight: number, keyPrefix: string) => {
    // Clamp to the chart so an ancestor spanning far beyond the focus window
    // renders as a full-width context bar instead of extreme off-screen coords.
    const x = Math.max(0, safeScale(xScale, task.start_time));
    const w = Math.max(1, Math.min(WIDTH, safeScale(xScale, task.end_time)) - x);
    const selected = selectedId != null && String(selectedId) === String(task.id);
    return (
      <g
        key={`${keyPrefix}-${task.id}`}
        className="cursor-pointer focus:outline-none"
        onClick={(event) => {
          event.stopPropagation();
          onSelectTask?.(task);
        }}
        onDoubleClick={() => onOpenTask?.(task)}
        onKeyDown={(event) => {
          if ((event.key === "Enter" || event.key === " ") && onOpenTask) {
            event.preventDefault();
            onOpenTask(task);
          }
        }}
        role={onOpenTask ? "link" : "button"}
        tabIndex={0}
      >
        <rect
          x={x}
          y={top}
          width={w}
          height={barHeight}
          fill={lookupColor(colorMap, task, colorMode)}
          stroke="#000000"
          strokeOpacity={selected ? 0.8 : 0.2}
          strokeWidth={1}
          opacity={selectedId == null || selected ? 1 : 0.6}
        />
        <title>
          {task.kind} - {task.what}
          {"\n"}
          {task.location}
          {"\n"}
          {smartString(task.start_time)} to {smartString(task.end_time)}
        </title>
      </g>
    );
  };

  const sectionLabel = (text: string, x: number, y: number) => (
    <text x={x} y={y} fontSize="12" fontWeight="600" fill="#0f172a" pointerEvents="none" stroke="#ffffff" strokeWidth={2.5} paintOrder="stroke">
      {text}
    </text>
  );

  return (
    <div className="h-full overflow-auto bg-white">
      <svg width="100%" viewBox={`0 0 ${WIDTH} ${height}`} className="min-w-[760px]" onClick={() => onDeselect?.()}>
        <defs>
          <pattern id="gantt-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
            <rect width="8" height="8" fill="rgba(148, 163, 184, 0.12)" />
            <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(100, 116, 139, 0.28)" strokeWidth="3" />
          </pattern>
        </defs>
        {gaps.map((gap, index) => {
          const x = safeScale(xScale, gap.start_time);
          const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
          return <rect key={index} x={x} y={MARGIN.top} width={w} height={height - MARGIN.top - MARGIN.bottom} fill="url(#gantt-gap-pattern)" />;
        })}

        {xTicks.map((tick) => (
          <g key={tick}>
            <line x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={MARGIN.top} y2={height - MARGIN.bottom} stroke="#cbd5e1" strokeDasharray="3 3" />
            <text x={safeScale(xScale, tick)} y={18} textAnchor="middle" fontSize="10" fill="#475569">
              {smartString(tick)}
            </text>
            <text x={safeScale(xScale, tick)} y={height - 8} textAnchor="middle" fontSize="10" fill="#475569">
              {smartString(tick)}
            </text>
          </g>
        ))}

        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={MARGIN.top} y2={MARGIN.top} stroke="#94a3b8" />
        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={height - MARGIN.bottom} y2={height - MARGIN.bottom} stroke="#94a3b8" />

        {ancestorRows.map(({ task, top }) => (
          <g key={`anc-${task.id}`}>
            {renderBar(task, top + (HEADER_ROW_HEIGHT - HEADER_BAR_H) / 2, HEADER_BAR_H, "anc")}
            {sectionLabel(task.kind, 12, top + 13)}
          </g>
        ))}

        {mainTask && currentTop != null && (
          <>
            {sectionLabel("Current Task", 12, currentTop + 13)}
            {renderBar(mainTask, currentTop + (HEADER_ROW_HEIGHT - HEADER_BAR_H) / 2, HEADER_BAR_H, "main")}
          </>
        )}

        {levelRows.map(({ index, tasks, labelTop, tasksTop }) => (
          <g key={`lvl-${index}`}>
            {sectionLabel(`Subtasks · L${index + 1}`, 12, labelTop + 12)}
            {tasks.map((task) => renderBar(task, tasksTop + (task.yIndex ?? 0) * ROW_HEIGHT, DESC_BAR_H, `lvl${index}`))}
          </g>
        ))}

        {milestoneMarks}
      </svg>

      {canExpand && (
        <div className="px-3 py-2">
          <button
            type="button"
            onClick={() => onExpandNext?.()}
            disabled={expanding}
            className="rounded border px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted disabled:opacity-50"
          >
            {expanding ? "Expanding…" : "Expand next level"}
          </button>
        </div>
      )}
    </div>
  );
}
