import * as d3 from "d3";
import { formatSI } from "../../utils/siFormat";
import { AXIS_LABEL_FONT_SIZE, COLOR_AXIS_LABEL, COLOR_GRID, GRID_DASH, GRID_OPACITY, safeScale } from "./chartStyle";

interface TimeTicksProps {
  ticks: number[];
  xScale: d3.ScaleLinear<number, number>;
  // Gridline vertical extent.
  gridTop: number;
  gridBottom: number;
  // Tick-label baselines: above and/or below the gridlines (omit for none).
  topLabelY?: number;
  bottomLabelY?: number;
  // Draw short solid tick marks at the gridline ends.
  tickMarks?: boolean;
}

// Time-axis gridlines + tick marks + labels with the one shared style (dashed
// #000 @0.3 gridlines, #475569 10px SI labels). Each chart passes only geometry
// (where the axis sits, whether labels go on top/bottom), so the look is defined
// once and can't drift between the stacked component charts and the task gantt.
export default function TimeTicks({
  ticks,
  xScale,
  gridTop,
  gridBottom,
  topLabelY,
  bottomLabelY,
  tickMarks = false,
}: TimeTicksProps) {
  return (
    <>
      {ticks.map((tick) => {
        const tx = safeScale(xScale, tick);
        return (
          <g key={tick} pointerEvents="none">
            <line x1={tx} x2={tx} y1={gridTop} y2={gridBottom} stroke={COLOR_GRID} strokeDasharray={GRID_DASH} opacity={GRID_OPACITY} />
            {tickMarks && <line x1={tx} x2={tx} y1={gridTop} y2={gridTop + 5} stroke={COLOR_GRID} />}
            {tickMarks && <line x1={tx} x2={tx} y1={gridBottom - 5} y2={gridBottom} stroke={COLOR_GRID} />}
            {topLabelY != null && (
              <text x={tx} y={topLabelY} textAnchor="middle" fontSize={AXIS_LABEL_FONT_SIZE} fill={COLOR_AXIS_LABEL}>
                {formatSI(tick)}
              </text>
            )}
            {bottomLabelY != null && (
              <text x={tx} y={bottomLabelY} textAnchor="middle" fontSize={AXIS_LABEL_FONT_SIZE} fill={COLOR_AXIS_LABEL}>
                {formatSI(tick)}
              </text>
            )}
          </g>
        );
      })}
    </>
  );
}
