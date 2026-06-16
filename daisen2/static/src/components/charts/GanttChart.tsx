import { useMemo } from "react";
import * as d3 from "d3";
import type { Segment, Task } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMap, lookupColor } from "../../utils/taskColorCoder";
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

  const timeStart = layout.length ? Math.min(...layout.map((task) => task.start_time)) : 0;
  const timeEnd = layout.length ? Math.max(...layout.map((task) => task.end_time)) : 1;
  const padding = (timeEnd - timeStart) * 0.02 || 1e-12;
  const startTime = timeStart - padding;
  const endTime = timeEnd + padding;
  const laneCount = Math.max(1, Math.max(...layout.map((task) => task.yIndex ?? 0), 0) + 1);
  const height = MARGIN.top + MARGIN.bottom + HEADER_ROW_HEIGHT * (parentTask ? 2 : mainTask ? 1 : 0) + laneCount * ROW_HEIGHT + 28;
  const width = 1200;
  const innerWidth = width - MARGIN.left - MARGIN.right;
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([MARGIN.left, width - MARGIN.right]);
  const colorMap = buildColorMap(layout);
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
      y += HEADER_ROW_HEIGHT + 8;
    }
    return y + (task.yIndex ?? 0) * ROW_HEIGHT;
  };

  if (!layout.length) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No tasks to display.</div>;
  }

  return (
    <div className="h-full overflow-auto bg-white">
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
        <text x={12} y={MARGIN.top + (parentTask ? HEADER_ROW_HEIGHT + 8 : 0) + (mainTask ? HEADER_ROW_HEIGHT + 8 : 0) + 16} fontSize="12" fontWeight="600" fill="#0f172a">
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

        {mainTask?.steps?.map((step, index) => (
          <circle
            key={`${step.kind}-${index}`}
            cx={safeScale(xScale, step.time)}
            cy={yForTask({ ...mainTask, isMainTask: true }) + 9}
            r={3}
            fill="#dc2626"
            stroke="#ffffff"
          >
            <title>
              {step.kind}: {step.what} at {smartString(step.time)}
            </title>
          </circle>
        ))}
      </svg>
    </div>
  );
}
