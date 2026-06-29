import * as d3 from "d3";
import type { Segment } from "../../types/task";
import { safeScale } from "./chartStyle";

// The "no trace collected" diagonal hatch, shared by every chart so the pattern
// reads the same everywhere. Define it once per <svg> with a unique id, then draw
// the gap rects referencing that id.
export function GapHatchDef({ id }: { id: string }) {
  return (
    <pattern id={id} patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
      <rect width="8" height="8" fill="rgba(128, 128, 128, 0.15)" />
      <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(128, 128, 128, 0.3)" strokeWidth="4" />
    </pattern>
  );
}

export function GapRects({
  gaps,
  xScale,
  patternId,
  top,
  height,
}: {
  gaps: Segment[];
  xScale: d3.ScaleLinear<number, number>;
  patternId: string;
  top: number;
  height: number;
}) {
  return (
    <>
      {gaps.map((gap, index) => {
        const x = safeScale(xScale, gap.start_time);
        const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
        return <rect key={index} x={x} y={top} width={w} height={height} fill={`url(#${patternId})`} pointerEvents="none" />;
      })}
    </>
  );
}
