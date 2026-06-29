import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import * as d3 from "d3";
import type { Segment, Task } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../../utils/taskColorCoder";
import type { ColorMode } from "../../utils/taskColorCoder";
import { milestonesOf, wavyPath, type SelectedMilestone } from "../../utils/milestoneViz";
import { formatSI } from "../../utils/siFormat";
import { useElementSize } from "../../hooks/useElementSize";

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
  // The selected blocking milestone (mutually exclusive with a selected task) and
  // legend-driven highlights — shared with the component view for consistency.
  selectedMilestone?: SelectedMilestone | null;
  highlightedKey?: string | null;
  highlightedReason?: string | null;
  onSelectTask?: (task: Task) => void;
  onOpenTask?: (task: Task) => void;
  onSelectMilestone?: (milestone: SelectedMilestone | null) => void;
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
// Bar/band heights aligned with the component view (TASK_VIEW_LARGE_TASK_HEIGHT
// 15, milestone band 18, subtask bar ~7) so the same rows read the same size.
const MILESTONE_BAND = 18;
const LABEL_H = 18;
const HEADER_BAR_H = 15;
const DESC_BAR_H = 7;

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
  selectedMilestone = null,
  highlightedKey = null,
  highlightedReason = null,
  onSelectTask,
  onOpenTask,
  onSelectMilestone,
  onDeselect,
  canExpand = false,
  expanding = false,
  onExpandNext,
}: GanttChartProps) {
  // Measure the scroll container so the chart can use pixel coordinates and fill
  // the available width and height (rather than aspect-scaling a fixed viewBox,
  // which left empty space below when there were few layers).
  const { ref: containerRef, size } = useElementSize<HTMLDivElement>();
  const W = Math.max(size.width || 1200, 760);
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

  // Scale time to the current task and its descendants (the focus). Ancestors run
  // far wider (the root spans the whole trace), so including them would squash the
  // focus into a sliver; instead they are clamped to the chart as context bars.
  const focusTasks = mainTask ? [mainTask, ...levels.flat()] : allTasks;
  const timeStart = focusTasks.length ? Math.min(...focusTasks.map((task) => task.start_time)) : 0;
  const timeEnd = focusTasks.length ? Math.max(...focusTasks.map((task) => task.end_time)) : 1;
  const padding = (timeEnd - timeStart) * 0.02 || 1e-12;
  const autoStart = timeStart - padding;
  const autoEnd = timeEnd + padding;

  // The visible time range starts at the auto-fit focus domain and resets when the
  // focus task changes. Drag pans (and scrolls vertically) and Ctrl/Cmd+scroll
  // zooms (anchored at the cursor) — matching the component view.
  const [viewRange, setViewRange] = useState({ startTime: autoStart, endTime: autoEnd });
  const rangeRef = useRef(viewRange);
  rangeRef.current = viewRange;
  const widthRef = useRef(W);
  widthRef.current = W;
  const dragRef = useRef<{ x: number; y: number; scrollTop: number; range: { startTime: number; endTime: number } } | null>(null);
  const didDragRef = useRef(false);
  const focusKey = mainTask?.id ?? "";
  useEffect(() => {
    setViewRange({ startTime: autoStart, endTime: autoEnd });
  }, [focusKey, autoStart, autoEnd]);
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const r = rangeRef.current;
      const rect = el.getBoundingClientRect();
      const inner = Math.max(1, widthRef.current - MARGIN.left - MARGIN.right);
      const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left - MARGIN.left) / inner));
      const dur = r.endTime - r.startTime;
      const scale = Math.pow(1.0015, event.deltaY);
      const anchor = r.startTime + dur * ratio;
      setViewRange({ startTime: anchor - (anchor - r.startTime) * scale, endTime: anchor + (r.endTime - anchor) * scale });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [containerRef]);
  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    dragRef.current = { x: event.clientX, y: event.clientY, scrollTop: containerRef.current?.scrollTop ?? 0, range: rangeRef.current };
    didDragRef.current = false;
  };
  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag) return;
    const dx = event.clientX - drag.x;
    const dy = event.clientY - drag.y;
    if (Math.abs(dx) > 2 || Math.abs(dy) > 2) didDragRef.current = true;
    const dur = drag.range.endTime - drag.range.startTime;
    const dt = (dur / Math.max(1, widthRef.current - MARGIN.left - MARGIN.right)) * dx;
    setViewRange({ startTime: drag.range.startTime - dt, endTime: drag.range.endTime - dt });
    if (containerRef.current) containerRef.current.scrollTop = drag.scrollTop - dy;
  };
  const handlePointerUp = () => {
    dragRef.current = null;
  };

  if (allTasks.length === 0) {
    // Keep the same ref'd container so the ResizeObserver stays attached (and the
    // measured size is ready) across the empty → loaded transition.
    return (
      <div ref={containerRef} className="h-full overflow-auto bg-white">
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No tasks to display.</div>
      </div>
    );
  }

  const startTime = viewRange.startTime;
  const endTime = viewRange.endTime;
  const innerWidth = W - MARGIN.left - MARGIN.right;
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([MARGIN.left, W - MARGIN.right]);
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
  const naturalHeight = cursor + MARGIN.bottom + 8;

  // Fill the container: when the content is shorter than the available height,
  // stretch the band layout (positions and bar heights, not label fonts) so the
  // chart uses the full vertical space and the bottom axis sits at the bottom,
  // matching the component view. Taller content scrolls at its natural size.
  const topAnchor = MARGIN.top + 8;
  const height = size.height > naturalHeight ? size.height : naturalHeight;
  const innerNatural = Math.max(1, naturalHeight - topAnchor - MARGIN.bottom - 8);
  const stretch = (height - topAnchor - MARGIN.bottom - 8) / innerNatural;
  const sy = (y: number) => topAnchor + (y - topAnchor) * stretch;
  const sh = (h: number) => h * stretch;

  const xTicks = xScale.ticks(12);
  const gaps = segmentsEnabled ? gapSegments(segments, startTime, endTime) : [];

  // Blocking-reason curves for the current task: each milestone closes an interval
  // [previous milestone (or task start) → milestone], drawn as a wavy arc colored
  // by the released reason with a node at the release point.
  const milestoneCenterY = currentTop != null ? sy(currentTop + HEADER_BAR_H + 6) : 0;
  const milestoneMarks =
    mainTask && currentTop != null
      ? milestoneSteps.map((step, index) => {
          const intervalStart = index === 0 ? mainTask.start_time : milestoneSteps[index - 1].time;
          const x0 = safeScale(xScale, intervalStart);
          const x1 = safeScale(xScale, step.time);
          const color = colorMap[step.kind] ?? "#9ca3af";
          const d = wavyPath(x0, x1, milestoneCenterY);
          // Click-to-select with the same affordance as the component view: the
          // selected milestone thickens with a ringed dot; a selection or a
          // highlighted reason dims the rest.
          const selected = selectedMilestone != null && selectedMilestone.kind === step.kind && selectedMilestone.time === step.time;
          const dimmed = (selectedMilestone != null && !selected) || (highlightedReason != null && highlightedReason !== step.kind);
          const opacity = dimmed ? 0.25 : 1;
          return (
            <g
              key={`milestone-${index}-${step.kind}`}
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                if (didDragRef.current) return;
                onSelectMilestone?.({ kind: step.kind, what: step.what, time: step.time, blockedFor: step.time - intervalStart });
              }}
            >
              {x1 - x0 >= 2 && (
                <>
                  <rect x={x0} y={milestoneCenterY - 8} width={x1 - x0} height={16} fill="transparent" pointerEvents="all" />
                  <path d={d} fill="none" stroke={color} strokeWidth={selected ? 3 : 1.5} strokeLinecap="round" opacity={opacity} pointerEvents="none" />
                </>
              )}
              {selected && <circle cx={x1} cy={milestoneCenterY} r={6} fill="none" stroke={color} strokeWidth={1.5} pointerEvents="none" />}
              <circle cx={x1} cy={milestoneCenterY} r={selected ? 3.5 : 3} fill={color} stroke="#ffffff" strokeWidth={0.75} opacity={opacity} pointerEvents="none" />
            </g>
          );
        })
      : null;

  const renderBar = (task: Task, top: number, barHeight: number, keyPrefix: string) => {
    // Clamp to the chart so an ancestor spanning far beyond the focus window
    // renders as a full-width context bar instead of extreme off-screen coords.
    const x = Math.max(0, safeScale(xScale, task.start_time));
    const w = Math.max(1, Math.min(W, safeScale(xScale, task.end_time)) - x);
    const key = taskColorKey(task, colorMode);
    const selected = selectedId != null && String(selectedId) === String(task.id);
    const hasHighlight = highlightedKey != null;
    const highlighted = hasHighlight && highlightedKey === key;
    return (
      <g
        key={`${keyPrefix}-${task.id}`}
        className="cursor-pointer"
        onClick={(event) => {
          event.stopPropagation();
          if (didDragRef.current) return;
          onSelectTask?.(task);
        }}
        onDoubleClick={() => onOpenTask?.(task)}
      >
        <rect
          x={x}
          y={top}
          width={w}
          height={barHeight}
          fill={lookupColor(colorMap, task, colorMode)}
          stroke="#000000"
          strokeOpacity={selected || highlighted ? 0.8 : 0.2}
          strokeWidth={1}
          opacity={hasHighlight ? (highlighted ? 1 : 0.4) : selectedId != null && !selected ? 0.6 : 1}
        />
      </g>
    );
  };

  // Section labels: 12px black with a white halo (paintOrder stroke) so they stay
  // readable over bars — the same treatment as the component view's row labels.
  const sectionLabel = (text: string, x: number, y: number) => (
    <text x={x} y={y} fontSize="12" fill="#000" stroke="#ffffff" strokeWidth={3} paintOrder="stroke" pointerEvents="none">
      {text}
    </text>
  );

  return (
    <div
      ref={containerRef}
      className="h-full cursor-grab overflow-auto bg-white active:cursor-grabbing"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerLeave={handlePointerUp}
    >
      <svg
        width={W}
        height={height}
        className="block"
        onClick={() => {
          if (didDragRef.current) return;
          onDeselect?.();
        }}
      >
        <defs>
          <pattern id="gantt-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
            <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
            <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
          </pattern>
        </defs>
        {gaps.map((gap, index) => {
          const x = safeScale(xScale, gap.start_time);
          const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
          return <rect key={index} x={x} y={MARGIN.top} width={w} height={height - MARGIN.top - MARGIN.bottom} fill="url(#gantt-gap-pattern)" />;
        })}

        {xTicks.map((tick) => {
          const tx = safeScale(xScale, tick);
          return (
            <g key={tick}>
              <line x1={tx} x2={tx} y1={MARGIN.top} y2={height - MARGIN.bottom} stroke="#000" strokeDasharray="3,3" opacity={0.3} />
              <line x1={tx} x2={tx} y1={MARGIN.top} y2={MARGIN.top + 5} stroke="#000" />
              <line x1={tx} x2={tx} y1={height - MARGIN.bottom - 5} y2={height - MARGIN.bottom} stroke="#000" />
              <text x={tx} y={18} textAnchor="middle" fontSize="10" fill="#475569">
                {formatSI(tick)}
              </text>
              <text x={tx} y={height - 8} textAnchor="middle" fontSize="10" fill="#475569">
                {formatSI(tick)}
              </text>
            </g>
          );
        })}

        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={MARGIN.top} y2={MARGIN.top} stroke="#000" />
        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={height - MARGIN.bottom} y2={height - MARGIN.bottom} stroke="#000" />

        {ancestorRows.map(({ task, top }) => (
          <g key={`anc-${task.id}`}>
            {renderBar(task, sy(top + (HEADER_ROW_HEIGHT - HEADER_BAR_H) / 2), sh(HEADER_BAR_H), "anc")}
            {sectionLabel(task.kind, 12, sy(top + 13))}
          </g>
        ))}

        {mainTask && currentTop != null && (
          <>
            {sectionLabel("Current Task", 12, sy(currentTop + 13))}
            {renderBar(mainTask, sy(currentTop + (HEADER_ROW_HEIGHT - HEADER_BAR_H) / 2), sh(HEADER_BAR_H), "main")}
          </>
        )}

        {levelRows.map(({ index, tasks, labelTop, tasksTop }) => (
          <g key={`lvl-${index}`}>
            {sectionLabel(`Subtasks · L${index + 1}`, 12, sy(labelTop + 12))}
            {tasks.map((task) => renderBar(task, sy(tasksTop + (task.yIndex ?? 0) * ROW_HEIGHT), sh(DESC_BAR_H), `lvl${index}`))}
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
