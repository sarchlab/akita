import * as d3 from "d3";
import { safeScale } from "./chartStyle";

// Roughly one repeated y-value label per this many pixels of chart width.
const Y_LABEL_SPACING = 450;

function formatCount(n: number): string {
  const abs = Math.abs(n);
  if (abs >= 1e9) return `${+(n / 1e9).toFixed(1)}B`;
  if (abs >= 1e6) return `${+(n / 1e6).toFixed(1)}M`;
  if (abs >= 1e3) return `${+(n / 1e3).toFixed(1)}k`;
  return String(n);
}

// YAxisOverlay draws the count gridlines and haloed value labels for a stacked/
// area chart, repeating the label across the width so it stays readable on a wide
// chart. Shared by the component overview charts and the resource page so the axis
// reads identically.
export default function YAxisOverlay({
  yScale,
  width,
  tickCount = 4,
}: {
  yScale: d3.ScaleLinear<number, number>;
  width: number;
  tickCount?: number;
}) {
  const left = 5;
  const right = Math.max(left + 1, width - 5);
  const intervals = Math.max(1, Math.round((right - left) / Y_LABEL_SPACING));
  const columns = Array.from({ length: intervals + 1 }, (_, i) => left + (i / intervals) * (right - left));
  // Skip the 0 baseline (it's implicit at the axis) and any non-integer ticks a
  // tiny range would produce.
  const ticks = yScale.ticks(tickCount).filter((tick) => Number.isInteger(tick) && tick > 0);
  return (
    <g pointerEvents="none">
      {ticks.map((tick) => {
        const y = safeScale(yScale, tick);
        const labelY = Math.max(9, y - 3);
        return (
          <g key={tick}>
            <line x1={left} x2={right} y1={y} y2={y} stroke="#94a3b8" strokeDasharray="3,3" opacity={0.45} />
            {columns.map((cx, i) => (
              <text
                key={i}
                x={cx}
                y={labelY}
                textAnchor={i === 0 ? "start" : i === columns.length - 1 ? "end" : "middle"}
                fontSize="10"
                fill="#475569"
                stroke="#ffffff"
                strokeWidth={2.5}
                paintOrder="stroke"
              >
                {formatCount(tick)}
              </text>
            ))}
          </g>
        );
      })}
    </g>
  );
}
