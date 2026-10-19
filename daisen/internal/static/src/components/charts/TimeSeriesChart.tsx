import * as d3 from "d3";
import { useRef } from "react";
import type { MouseEvent, PointerEvent, WheelEvent } from "react";
import type { ComponentInfo } from "../../hooks/useCompInfo";
import type { Segment } from "../../types/task";
import { formatSI } from "../../utils/siFormat";
import { formatVirtualTime } from "../../lib/time";

interface Series {
  info: ComponentInfo | null;
  color: string;
  side: "left" | "right";
}

interface TimeSeriesChartProps {
  width: number;
  height: number;
  startTime: number;
  endTime: number;
  series: Series[];
  segments?: Segment[];
  segmentsEnabled?: boolean;
  onTimeRangeChange?: (range: { startTime: number; endTime: number }) => void;
}

const MARGIN = { top: 10, right: 54, bottom: 26, left: 54 };

function safeScale(scale: d3.ScaleLinear<number, number>, value: number) {
  return scale(value) ?? 0;
}

function yDomain(info: ComponentInfo | null) {
  const max = d3.max(info?.data ?? [], (d) => d.value) ?? 0;
  return [0, max || 1] as [number, number];
}

function nonTracedPeriods(segments: Segment[], startTime: number, endTime: number) {
  if (!segments.length) return [];
  const sorted = [...segments].sort((a, b) => a.start_time - b.start_time);
  const gaps: Segment[] = [];
  if (sorted[0].start_time > startTime) {
    gaps.push({ start_time: startTime, end_time: Math.min(sorted[0].start_time, endTime) });
  }
  for (let i = 0; i < sorted.length - 1; i++) {
    const start = Math.max(sorted[i].end_time, startTime);
    const end = Math.min(sorted[i + 1].start_time, endTime);
    if (start < end) gaps.push({ start_time: start, end_time: end });
  }
  const last = sorted[sorted.length - 1];
  if (last.end_time < endTime) {
    gaps.push({ start_time: Math.max(last.end_time, startTime), end_time: endTime });
  }
  return gaps;
}

export default function TimeSeriesChart({
  width,
  height,
  startTime,
  endTime,
  series,
  segments = [],
  segmentsEnabled = false,
  onTimeRangeChange,
}: TimeSeriesChartProps) {
  const dragRef = useRef<{ pointerId: number; x: number; startTime: number; endTime: number } | null>(null);
  const didDragRef = useRef(false);
  const innerWidth = Math.max(1, width - MARGIN.left - MARGIN.right);
  const innerHeight = Math.max(1, height - MARGIN.top - MARGIN.bottom);
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([0, innerWidth]);
  const xTicks = xScale.ticks(5);

  const visibleSeries = series.filter((entry) => entry.info?.data?.length);
  const leftScale = d3.scaleLinear().domain(yDomain(series.find((s) => s.side === "left")?.info ?? null)).nice().range([innerHeight, 0]);
  const rightScale = d3.scaleLinear().domain(yDomain(series.find((s) => s.side === "right")?.info ?? null)).nice().range([innerHeight, 0]);

  const lineFor = (entry: Series) => {
    const yScale = entry.side === "left" ? leftScale : rightScale;
    return d3
      .line<{ time: number; value: number }>()
      .x((d) => safeScale(xScale, d.time))
      .y((d) => safeScale(yScale, d.value))
      .curve(d3.curveCatmullRom.alpha(0.5))(entry.info?.data ?? []);
  };

  const gaps = segmentsEnabled ? nonTracedPeriods(segments, startTime, endTime) : [];

  const handleWheel = (event: WheelEvent<SVGRectElement>) => {
    if (!onTimeRangeChange) return;

    event.preventDefault();
    event.stopPropagation();

    const duration = endTime - startTime;
    if (!Number.isFinite(duration) || duration <= 0) return;

    const bounds = event.currentTarget.getBoundingClientRect();
    const pointerX = Math.min(Math.max(event.clientX - bounds.left, 0), bounds.width);
    const pointerRatio = bounds.width > 0 ? pointerX / bounds.width : 0.5;
    let nextStartTime = startTime;
    let nextEndTime = endTime;

    if (event.deltaY !== 0) {
      const scale = Math.pow(1.001, event.deltaY);
      const pointerTime = startTime + duration * pointerRatio;
      nextStartTime = pointerTime - (pointerTime - startTime) * scale;
      nextEndTime = pointerTime + (endTime - pointerTime) * scale;
    }

    if (event.deltaX !== 0) {
      const shift = (nextEndTime - nextStartTime) * event.deltaX * 0.001;
      nextStartTime += shift;
      nextEndTime += shift;
    }

    if (Number.isFinite(nextStartTime) && Number.isFinite(nextEndTime) && nextEndTime > nextStartTime) {
      onTimeRangeChange({ startTime: nextStartTime, endTime: nextEndTime });
    }
  };

  const handlePointerDown = (event: PointerEvent<SVGRectElement>) => {
    if (!onTimeRangeChange || event.button !== 0) return;

    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      startTime,
      endTime,
    };
    didDragRef.current = false;
  };

  const handlePointerMove = (event: PointerEvent<SVGRectElement>) => {
    const drag = dragRef.current;
    if (!onTimeRangeChange || !drag || drag.pointerId !== event.pointerId) return;

    event.preventDefault();
    const duration = drag.endTime - drag.startTime;
    if (!Number.isFinite(duration) || duration <= 0 || innerWidth <= 0) return;

    const pixelDelta = event.clientX - drag.x;
    if (Math.abs(pixelDelta) > 2) {
      didDragRef.current = true;
    }

    const timeDelta = (duration / innerWidth) * pixelDelta;
    onTimeRangeChange({
      startTime: drag.startTime - timeDelta,
      endTime: drag.endTime - timeDelta,
    });
  };

  const stopDragging = (event: PointerEvent<SVGRectElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    dragRef.current = null;
  };

  const handleClick = (event: MouseEvent<SVGRectElement>) => {
    if (!didDragRef.current) return;

    event.preventDefault();
    event.stopPropagation();
    didDragRef.current = false;
  };

  return (
    <svg width={width} height={height} className="block overflow-visible">
      <defs>
        <pattern id="widget-gap-pattern" patternUnits="userSpaceOnUse" width="8" height="8" patternTransform="rotate(45)">
          <rect width="8" height="8" fill="rgba(148, 163, 184, 0.12)" />
          <line x1="0" y1="0" x2="0" y2="8" stroke="rgba(100, 116, 139, 0.28)" strokeWidth="3" />
        </pattern>
      </defs>
      <g transform={`translate(${MARGIN.left}, ${MARGIN.top})`}>
        {gaps.map((gap, index) => {
          const x = safeScale(xScale, gap.start_time);
          const w = Math.max(0, safeScale(xScale, gap.end_time) - x);
          return <rect key={index} x={x} y={0} width={w} height={innerHeight} fill="url(#widget-gap-pattern)" />;
        })}

        {xTicks.map((tick) => (
          <line key={tick} x1={safeScale(xScale, tick)} x2={safeScale(xScale, tick)} y1={0} y2={innerHeight} stroke="#cbd5e1" strokeDasharray="3 3" />
        ))}

        <g className="chart-axis">
          {leftScale.ticks(4).map((tick) => (
            <g key={tick} transform={`translate(0, ${safeScale(leftScale, tick)})`}>
              <line x1="-4" x2="0" stroke="#94a3b8" />
              <text x="-8" dy="0.32em" textAnchor="end">
                {formatSI(tick)}
              </text>
            </g>
          ))}
          {rightScale.ticks(4).map((tick) => (
            <g key={tick} transform={`translate(${innerWidth}, ${safeScale(rightScale, tick)})`}>
              <line x1="0" x2="4" stroke="#94a3b8" />
              <text x="8" dy="0.32em">
                {formatSI(tick)}
              </text>
            </g>
          ))}
          {xTicks.map((tick) => (
            <g key={tick} transform={`translate(${safeScale(xScale, tick)}, ${innerHeight})`}>
              <line y2="4" stroke="#94a3b8" />
              <text y="17" textAnchor="middle">
                {formatVirtualTime(tick)}
              </text>
            </g>
          ))}
          <line x1={0} x2={innerWidth} y1={innerHeight} y2={innerHeight} stroke="#94a3b8" />
          <line x1={0} x2={0} y1={0} y2={innerHeight} stroke="#94a3b8" />
          <line x1={innerWidth} x2={innerWidth} y1={0} y2={innerHeight} stroke="#94a3b8" />
        </g>

        {visibleSeries.map((entry) => (
          <g key={`${entry.info?.info_type}-${entry.side}`}>
            <path d={lineFor(entry) ?? ""} fill="none" stroke={entry.color} strokeWidth="1.75" />
            {(entry.info?.data ?? []).map((point, index) => {
              const yScale = entry.side === "left" ? leftScale : rightScale;
              return (
                <circle
                  key={index}
                  cx={safeScale(xScale, point.time)}
                  cy={safeScale(yScale, point.value)}
                  r="2"
                  fill={entry.color}
                />
              );
            })}
          </g>
        ))}
        <rect
          x={0}
          y={0}
          width={innerWidth}
          height={innerHeight}
          fill="transparent"
          className={onTimeRangeChange ? "cursor-grab active:cursor-grabbing" : undefined}
          onWheel={handleWheel}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={stopDragging}
          onPointerCancel={stopDragging}
          onLostPointerCapture={() => {
            dragRef.current = null;
          }}
          onClick={handleClick}
          pointerEvents={onTimeRangeChange ? "all" : "none"}
        />
      </g>
    </svg>
  );
}
