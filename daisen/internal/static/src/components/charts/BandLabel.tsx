import { COLOR_HALO, SECTION_LABEL_FONT_SIZE, SECTION_LABEL_HALO_WIDTH } from "./chartStyle";

// A gantt row/section label drawn over the bars (e.g. "Current Task", an ancestor
// kind). One definition so the font, color, and white halo are identical on the
// component view's nested gantt and the task view's gantt.
export default function BandLabel({ x, y, children }: { x: number; y: number; children: string }) {
  return (
    <text
      x={x}
      y={y}
      fontSize={SECTION_LABEL_FONT_SIZE}
      fill="#000"
      stroke={COLOR_HALO}
      strokeWidth={SECTION_LABEL_HALO_WIDTH}
      paintOrder="stroke"
      textAnchor="start"
      pointerEvents="none"
      style={{ userSelect: "none" }}
    >
      {children}
    </text>
  );
}
