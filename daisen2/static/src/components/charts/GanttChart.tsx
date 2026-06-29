import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import * as d3 from "d3";
import type { Segment, Task } from "../../types/task";
import { assignYIndices } from "../../utils/taskYIndexAssigner";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../../utils/taskColorCoder";
import type { ColorMode } from "../../utils/taskColorCoder";
import { milestonesOf, type SelectedMilestone } from "../../utils/milestoneViz";
import { useElementSize } from "../../hooks/useElementSize";
import { AXIS_TICK_COUNT, barOpacity, barStrokeOpacity, COLOR_BAR_STROKE, COLOR_GRID, gapSegments, safeScale } from "./chartStyle";
import BandLabel from "./BandLabel";
import { GapHatchDef, GapRects } from "./GapHatch";
import MilestoneMarks from "./MilestoneMarks";
import TimeTicks from "./TimeTicks";

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
// Vertical room reserved below the current task bar for the blocking-reason
// curves (only when the task has milestones).
// Bar/band heights aligned with the component view (TASK_VIEW_LARGE_TASK_HEIGHT
// 15, milestone band 18, subtask bar ~7) so the same rows read the same size.
const MILESTONE_BAND = 18;
const LABEL_H = 18;
const HEADER_BAR_H = 15;
const DESC_BAR_H = 7;

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

  // Vertical layout, top → bottom. Each task has a label row above its bar; tasks
  // with milestones get a band below the bar for the blocking waves. Label-above
  // and milestones-below keep both off the bar, so the bar stays clickable.
  const ROW_GAP = 4;
  const hasMilestones = (task: Task) => milestonesOf(task.steps).length > 0;
  let cursor = MARGIN.top + 8;
  const ancestorRows = ancestors.map((task) => {
    const labelTop = cursor;
    const barTop = cursor + LABEL_H;
    const band = hasMilestones(task) ? MILESTONE_BAND : 0;
    cursor += LABEL_H + DESC_BAR_H + band + ROW_GAP;
    return { task, labelTop, barTop, band };
  });
  const currentLabelTop = cursor;
  const currentBarTop = cursor + LABEL_H;
  if (mainTask) cursor += LABEL_H + HEADER_BAR_H + milestoneBand + ROW_GAP;
  const levelRows = levelLayouts.map(({ tasks, lanes }, index) => {
    const labelTop = cursor;
    cursor += LABEL_H;
    const tasksTop = cursor;
    const levelHasMs = tasks.some(hasMilestones);
    const laneH = ROW_HEIGHT + (levelHasMs ? MILESTONE_BAND : 0);
    cursor += lanes * laneH + 6;
    return { index, tasks, labelTop, tasksTop, laneH, levelHasMs };
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

  const xTicks = xScale.ticks(AXIS_TICK_COUNT);
  const gaps = segmentsEnabled ? gapSegments(segments, startTime, endTime) : [];

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
          stroke={COLOR_BAR_STROKE}
          strokeWidth={1}
          strokeOpacity={barStrokeOpacity({ selected, highlighted, hasHighlight })}
          opacity={barOpacity({ selected, highlighted, hasHighlight, hasSelection: selectedId != null })}
        />
      </g>
    );
  };

  // Milestone waves for any listed task, drawn centered on its bar. The current
  // task gets its own dedicated band below the bar (milestoneMarks above); every
  // other task shows its milestones over its bar so all blocking is visible.
  const renderTaskMilestones = (task: Task, centerY: number, keyPrefix: string) => {
    const steps = milestonesOf(task.steps).sort((a, b) => a.time - b.time);
    if (!steps.length) return null;
    return (
      <MilestoneMarks
        key={keyPrefix}
        steps={steps}
        taskStart={task.start_time}
        xScale={xScale}
        centerY={centerY}
        colorMap={colorMap}
        selectedMilestone={selectedMilestone}
        highlightedReason={highlightedReason}
        onSelect={(milestone) => {
          if (didDragRef.current) return;
          onSelectMilestone?.(milestone);
        }}
      />
    );
  };

  return (
    <div
      ref={containerRef}
      className="h-full cursor-grab select-none overflow-auto bg-white active:cursor-grabbing"
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
          <GapHatchDef id="gantt-gap-pattern" />
        </defs>
        <GapRects gaps={gaps} xScale={xScale} patternId="gantt-gap-pattern" top={MARGIN.top} height={height - MARGIN.top - MARGIN.bottom} />

        <TimeTicks
          ticks={xTicks}
          xScale={xScale}
          gridTop={MARGIN.top}
          gridBottom={height - MARGIN.bottom}
          topLabelY={18}
          bottomLabelY={height - 8}
          tickMarks
        />

        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={MARGIN.top} y2={MARGIN.top} stroke={COLOR_GRID} />
        <line x1={MARGIN.left} x2={MARGIN.left + innerWidth} y1={height - MARGIN.bottom} y2={height - MARGIN.bottom} stroke={COLOR_GRID} />

        {ancestorRows.map(({ task, labelTop, barTop, band }) => (
          <g key={`anc-${task.id}`}>
            <BandLabel x={12} y={sy(labelTop + 12)}>{task.kind}</BandLabel>
            {renderBar(task, sy(barTop), sh(DESC_BAR_H), "anc")}
            {band > 0 && renderTaskMilestones(task, sy(barTop + DESC_BAR_H + MILESTONE_BAND / 2), `anc-ms-${task.id}`)}
          </g>
        ))}

        {mainTask && (
          <g>
            <BandLabel x={12} y={sy(currentLabelTop + 12)}>Current Task</BandLabel>
            {renderBar(mainTask, sy(currentBarTop), sh(HEADER_BAR_H), "main")}
            {milestoneBand > 0 && renderTaskMilestones(mainTask, sy(currentBarTop + HEADER_BAR_H + MILESTONE_BAND / 2), "main-ms")}
          </g>
        )}

        {levelRows.map(({ index, tasks, labelTop, tasksTop, laneH, levelHasMs }) => (
          <g key={`lvl-${index}`}>
            <BandLabel x={12} y={sy(labelTop + 12)}>{`Subtasks · L${index + 1}`}</BandLabel>
            {tasks.map((task) => renderBar(task, sy(tasksTop + (task.yIndex ?? 0) * laneH), sh(DESC_BAR_H), `lvl${index}`))}
            {levelHasMs &&
              tasks.map((task) =>
                renderTaskMilestones(task, sy(tasksTop + (task.yIndex ?? 0) * laneH + DESC_BAR_H + MILESTONE_BAND / 2), `lvl${index}-ms-${task.id}`),
              )}
          </g>
        ))}
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
