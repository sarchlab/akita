import React from "react";
import type { ColorMap } from "../utils/taskColorCoder";

interface TaskColorLegendProps {
  colorMap: ColorMap;
  onHighlight?: (kindWhat: string | null) => void;
}

/**
 * Renders a color legend for task kinds.
 * Each entry shows a colored swatch and the kind-what label.
 */
export default function TaskColorLegend({
  colorMap,
  onHighlight,
}: TaskColorLegendProps) {
  const entries = Object.entries(colorMap);

  if (entries.length === 0) return null;

  return (
    <div style={{ padding: "8px", fontSize: "12px" }}>
      <svg
        width="200"
        height={entries.length * 28 + 30}
        style={{ overflow: "visible" }}
      >
        {entries.map(([kindWhat, color], i) => (
          <g
            key={kindWhat}
            transform={`translate(5, ${i * 28 + 30})`}
            style={{ cursor: "pointer" }}
            onMouseEnter={() => onHighlight?.(kindWhat)}
            onMouseLeave={() => onHighlight?.(null)}
          >
            <rect
              y={-9}
              width={30}
              height={10}
              fill={color}
              stroke="black"
              strokeWidth={0.5}
            />
            <text x={40} dominantBaseline="middle" fontSize={11}>
              {kindWhat}
            </text>
          </g>
        ))}
      </svg>
    </div>
  );
}
