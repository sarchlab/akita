import * as d3 from "d3";
import type { TaskStep } from "../../types/task";
import { wavyPath, type SelectedMilestone } from "../../utils/milestoneViz";
import {
  COLOR_HALO,
  COLOR_TASK_FALLBACK,
  MILESTONE_DOT_R,
  MILESTONE_DOT_R_SELECTED,
  MILESTONE_HIT_HEIGHT,
  MILESTONE_RING_R,
  MILESTONE_WAVE_WIDTH,
  MILESTONE_WAVE_WIDTH_SELECTED,
  OPACITY_DIM_MILESTONE,
  safeScale,
} from "./chartStyle";

interface MilestoneMarksProps {
  // Milestones of the current task, sorted by time.
  steps: TaskStep[];
  // The current task's start time (the first interval starts here).
  taskStart: number;
  xScale: d3.ScaleLinear<number, number>;
  centerY: number;
  colorMap: Record<string, string>;
  selectedMilestone: SelectedMilestone | null;
  // A hovered/selected blocking reason; non-matching waves dim.
  highlightedReason: string | null;
  // Caller decides what selecting a milestone does (and guards drags). The hit
  // rect also carries data-ms-* so a pointer-capturing container can resolve the
  // click itself when the native onClick is swallowed.
  onSelect: (milestone: SelectedMilestone) => void;
}

// The blocking-milestone glyphs on a task's wave: an interval wave + release dot,
// with the shared selected affordance (thicken + ring, dim the rest). Shared by
// the component view's nested gantt and the task view's gantt so they match.
export default function MilestoneMarks({
  steps,
  taskStart,
  xScale,
  centerY,
  colorMap,
  selectedMilestone,
  highlightedReason,
  onSelect,
}: MilestoneMarksProps) {
  return (
    <>
      {steps.map((step, index) => {
        const intervalStart = index === 0 ? taskStart : steps[index - 1].time;
        const x0 = safeScale(xScale, intervalStart);
        const x1 = safeScale(xScale, step.time);
        const color = colorMap[step.kind] ?? COLOR_TASK_FALLBACK;
        const d = wavyPath(x0, x1, centerY);
        const selected =
          selectedMilestone != null && selectedMilestone.kind === step.kind && selectedMilestone.time === step.time;
        const dimmed = (selectedMilestone != null && !selected) || (highlightedReason != null && highlightedReason !== step.kind);
        const opacity = dimmed ? OPACITY_DIM_MILESTONE : 1;
        return (
          <g
            key={`milestone-${index}-${step.kind}`}
            className="cursor-pointer"
            onClick={(event) => {
              event.stopPropagation();
              onSelect({ kind: step.kind, what: step.what, time: step.time, blockedFor: step.time - intervalStart });
            }}
          >
            {x1 - x0 >= 2 && (
              <>
                <rect
                  x={x0}
                  y={centerY - MILESTONE_HIT_HEIGHT / 2}
                  width={x1 - x0}
                  height={MILESTONE_HIT_HEIGHT}
                  fill="transparent"
                  pointerEvents="all"
                  data-ms-kind={step.kind}
                  data-ms-what={step.what}
                  data-ms-time={step.time}
                  data-ms-blocked={step.time - intervalStart}
                />
                <path
                  d={d}
                  fill="none"
                  stroke={color}
                  strokeWidth={selected ? MILESTONE_WAVE_WIDTH_SELECTED : MILESTONE_WAVE_WIDTH}
                  strokeLinecap="round"
                  opacity={opacity}
                  pointerEvents="none"
                />
              </>
            )}
            {selected && (
              <circle cx={x1} cy={centerY} r={MILESTONE_RING_R} fill="none" stroke={color} strokeWidth={1.5} pointerEvents="none" />
            )}
            <circle
              cx={x1}
              cy={centerY}
              r={selected ? MILESTONE_DOT_R_SELECTED : MILESTONE_DOT_R}
              fill={color}
              stroke={COLOR_HALO}
              strokeWidth={0.75}
              opacity={opacity}
              pointerEvents="none"
            />
          </g>
        );
      })}
    </>
  );
}
