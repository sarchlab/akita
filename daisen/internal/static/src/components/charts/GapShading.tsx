import * as d3 from "d3";
import type { Segment } from "../../types/task";
import { GapHatchDef, GapRects } from "./GapHatch";

// GapShading hatches the time ranges where no trace was collected, so the overview
// charts and the resource page read consistently. Drawn on top of filled areas (it
// is faint) so a gap stays visible even over a band.
export default function GapShading({
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
        <GapHatchDef id={patternId} />
      </defs>
      <GapRects gaps={gaps} xScale={xScale} patternId={patternId} top={0} height={height} />
    </>
  );
}
