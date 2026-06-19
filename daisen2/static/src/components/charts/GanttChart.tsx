import { useMemo } from "react";
import * as d3 from "d3";
import type { Segment, Task } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../../utils/taskColorCoder";
import { wavyPath } from "../../utils/milestoneViz";
import { smartString } from "../../utils/smartValue";

interface GanttChartProps {
  tasks: Task[];
  mainTask?: Task | null;
  parentTask?: Task | null;
  segments?: Segment[];
  segmentsEnabled?: boolean;
  // Controlled selection: the parent owns which task is highlighted, so the
  // Gantt's highlight can't drift from the page's selected task / the URL.
  selectedId?: string | number | null;
  onSelectTask?: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
}

const MARGIN = { top: 28, right: 12, bottom: 28, left: 8 };
const ROW_HEIGHT = 14;
const HEADER_ROW_HEIGHT = 22;
// Vertical room reserved below the Current Task bar for the blocking-reason
// curves (only when the task has milestones).
const MILESTONE_BAND = 22;

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
  tasks,
  mainTask = null,
  parentTask = null,
  segments = [],
  segmentsEnabled = false,
  selectedId = null,
  onSelectTask,
  onOpenTask,
}: GanttChartProps) {
  const layout = useMemo(() => {
    const rows: Task[] = [];
    if (parentTask) rows.push({ ...parentTask, isParentTask: true });
    if (mainTask) rows.push({ ...mainTask, isMainTask: true });
    const ordinaryTasks = tasks
      .filter((task) => task.id !== parentTask?.id && task.id !== mainTask?.id)
      .map((task) => ({ ...task }));
    assignYIndices(ordinaryTasks);
    rows.push(...ordinaryTasks);
    return rows;
  }, [tasks, mainTask, parentTask]);

  // Milestones on the current task, in time order. Each is the release of a
  // blocking reason; the interval before it (from the task start or the previous
  // milestone) is rendered as a curve colored by that reason.
  const milestoneSteps = useMemo(() => {
    const steps = mainTask?.steps ?? [];
    return [...steps].sort((a, b) => a.time - b.time);
  }, [mainTask]);
  const milestoneBand = milestoneSteps.length ? MILESTONE_BAND : 0;

  const timeStart = layout.length ? Math.min(...layout.map((task) => task.start_time)) : 0;
  const timeEnd = layout.length ? Math.max(...layout.map((task) => task.end_time)) : 1;
  const padding = (timeEnd - timeStart) * 0.02 || 1e-12;
  const startTime = timeStart - padding;
  const endTime = timeEnd + padding;
  const laneCount = Math.max(1, Math.max(...layout.map((task) => task.yIndex ?? 0), 0) + 1);
  const height = MARGIN.top + MARGIN.bottom + HEADER_ROW_HEIGHT * (parentTask ? 2 : mainTask ? 1 : 0) + milestoneBand + laneCount * ROW_HEIGHT + 28;
  const width = 1200;
  const innerWidth = width - MARGIN.left - MARGIN.right;
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([MARGIN.left, width - MARGIN.right]);
  const colorMap = buildColorMapFromKeys([...layout.map(taskColorKey), ...milestoneSteps.map((step) => step.kind)]);
  const xTicks = xScale.ticks(12);
  const gaps = segmentsEnabled ? gapSegments(segments, startTime, endTime) : [];

  const yForTask = (task: Task) => {
    let y = MARGIN.top + 8;
    if (parentTask) {
      if (task.isParentTask) return y;
      y += HEADER_ROW_HEIGHT + 8;
    }
    if (mainTask) {
      if (task.isMainTask) return y;
      y += HEADER_ROW_HEIGHT + milestoneBand + 8;
    }
    return y + (task.yIndex ?? 0) * ROW_HEIGHT;
  };

  if (!layout.length) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No tasks to display.</div>;
  }

  // Blocking-reason curves for the current task. Each milestone closes an
  // interval [previous milestone (or task start) -> milestone]; that interval is
  // drawn as a downward arc colored by the released reason, with a node marking
  // the release point.
  const milestoneCenterY = mainTask ? yForTask({ ...mainTask, isMainTask: true }) + 18 + 6 : 0;
  const milestoneMarks = mainTask
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
                {/* Wide transparent overlay so the thin wave is easy to hover. */}
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

  const reasons = Array.from(new Set(milestoneSteps.map((step) => step.kind)));

  return (
    <div className="h-full overflow-auto bg-white">
      {reasons.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 pt-2 text-xs text-slate-600">
          <span className="font-medium">Blocked on:</span>
          {reasons.map((kind) => (
            <span key={kind} className="inline-flex items-center gap-1">
              <svg width="22" height="12" aria-hidden="true">
                <path
                  d={wavyPath(1, 21, 6, 3, 3)}
                  fill="none"
                  stroke={colorMap[kind] ?? "#9ca3af"}
                  strokeWidth={1.5}
                  strokeLinecap="round"
                />
              </svg>
              {kind}
            </span>
          ))}
        </div>
      )}
      <svg width="100%" viewBox={`0 0 ${width} ${height}`} className="min-w-[760px]">
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

        {parentTask && (
          <text x={12} y={yForTask({ ...parentTask, isParentTask: true }) + 13} fontSize="12" fontWeight="600" fill="#0f172a">
            Parent Task
          </text>
        )}
        {mainTask && (
          <text x={12} y={yForTask({ ...mainTask, isMainTask: true }) + 13} fontSize="12" fontWeight="600" fill="#0f172a">
            Current Task
          </text>
        )}
        <text x={12} y={MARGIN.top + (parentTask ? HEADER_ROW_HEIGHT + 8 : 0) + (mainTask ? HEADER_ROW_HEIGHT + milestoneBand + 8 : 0) + 16} fontSize="12" fontWeight="600" fill="#0f172a">
          Tasks
        </text>

        {layout.map((task) => {
          const x = safeScale(xScale, task.start_time);
          const w = Math.max(1, safeScale(xScale, task.end_time) - x);
          const selected = selectedId != null && String(selectedId) === String(task.id);
          return (
            <g
              key={`${task.id}-${task.isParentTask ? "parent" : task.isMainTask ? "main" : "task"}`}
              className="cursor-pointer"
              onClick={() => onSelectTask?.(task)}
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
                y={yForTask(task)}
                width={w}
                height={task.isParentTask || task.isMainTask ? 18 : 9}
                rx={2}
                fill={lookupColor(colorMap, task)}
                stroke={selected ? "#0f172a" : "rgba(15, 23, 42, 0.25)"}
                strokeWidth={selected ? 2 : 1}
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
        })}

        {milestoneMarks}
      </svg>
    </div>
  );
}
