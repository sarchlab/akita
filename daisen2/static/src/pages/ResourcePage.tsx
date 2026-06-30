import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { Minus, Plus } from "lucide-react";
import * as d3 from "d3";
import { useResourceBlocking } from "../hooks/useResourceBlocking";
import { useResourceTasks } from "../hooks/useResourceTasks";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useElementSize } from "../hooks/useElementSize";
import TraceChartLayout from "../components/TraceChartLayout";
import TimeTicks from "../components/charts/TimeTicks";
import { SectionLabel } from "../components/Legend";
import MilestoneMarks from "../components/charts/MilestoneMarks";
import { milestonesOf } from "../utils/milestoneViz";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../utils/taskColorCoder";
import type { Task } from "../types/task";
import {
  AXIS_LABEL_FONT_SIZE,
  COLOR_AXIS_LABEL,
  COLOR_BAR_STROKE,
  COLOR_GRID,
  barOpacity,
  barStrokeOpacity,
} from "../components/charts/chartStyle";

// When a resource blocks at most this many tasks, show a per-task gantt (each
// task's wait for the resource highlighted) instead of the density area.
const GANTT_THRESHOLD = 80;
const HW_RESOURCE_KIND = "hardware_resource";

// blockedIntervals returns each [lo, hi] span a task spent blocked on `what` — the
// interval ending at each matching milestone, from the previous milestone (or the
// task's start).
function blockedIntervals(task: Task, what: string): { lo: number; hi: number }[] {
  const ms = milestonesOf(task.steps).slice().sort((a, b) => a.time - b.time);
  const out: { lo: number; hi: number }[] = [];
  for (let i = 0; i < ms.length; i++) {
    if (ms[i].kind === HW_RESOURCE_KIND && ms[i].what === what) {
      out.push({ lo: i > 0 ? ms[i - 1].time : task.start_time, hi: ms[i].time });
    }
  }
  return out;
}

const MARGIN = { top: 26, right: 16, bottom: 26, left: 54 };
const MIN_RANGE = 1e-12;
const DEBOUNCE_MS = 400;
// Warm fill, matching the blocking-reason (milestone) color family.
const FILL = "#f59e0b";
const STROKE = "#ea580c";

interface TimeRange {
  startTime: number;
  endTime: number;
}

function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(id);
  }, [delayMs, value]);
  return debounced;
}

function sanitize(start: number, end: number): TimeRange {
  if (Number.isFinite(start) && Number.isFinite(end) && end > start) return { startTime: start, endTime: end };
  return { startTime: 0, endTime: MIN_RANGE };
}

// ResourcePage (route /resource?what=<name>) charts how many tasks are blocked on
// one hardware resource over time — the shaded-area occupancy, the same method the
// task-count chart uses. The curve's buildup and fall is where the resource's
// contention forms and resolves. Drag pans, ⌘/Ctrl+scroll (or the buttons) zoom,
// the time axis sits top and bottom, and a side panel carries the resource detail
// — matching the component and task views.
export default function ResourcePage() {
  const [searchParams] = useSearchParams();
  const what = searchParams.get("what") ?? "";
  const { startTime: simStart, endTime: simEnd } = useSimulationRange();

  // An explicit ?starttime/&endtime pins the range (shareable / back-compat);
  // otherwise the view follows the simulation range until the user pans/zooms.
  const urlStart = Number(searchParams.get("starttime"));
  const urlEnd = Number(searchParams.get("endtime"));
  const urlHasRange =
    searchParams.has("starttime") && Number.isFinite(urlStart) && Number.isFinite(urlEnd) && urlEnd > urlStart;
  const [viewRange, setViewRange] = useState<TimeRange>(
    urlHasRange ? { startTime: urlStart, endTime: urlEnd } : { startTime: simStart, endTime: simEnd },
  );
  const [userZoomed, setUserZoomed] = useState(urlHasRange);
  useEffect(() => {
    if (!userZoomed) setViewRange({ startTime: simStart, endTime: simEnd });
  }, [simStart, simEnd, userZoomed]);

  const dataRange = useDebouncedValue(viewRange, DEBOUNCE_MS);
  const dataPending =
    viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;

  // Mirror the (debounced) range into the URL — like the component page — so the
  // view is shareable and survives a refresh. replaceState avoids a history entry
  // (and a re-render) per zoom step.
  useEffect(() => {
    if (!what) return;
    const params = new URLSearchParams(window.location.search);
    params.set("what", what);
    params.set("starttime", dataRange.startTime.toString());
    params.set("endtime", dataRange.endTime.toString());
    window.history.replaceState(null, "", `/resource?${params.toString()}`);
  }, [dataRange.startTime, dataRange.endTime, what]);

  const { ref, size } = useElementSize<HTMLDivElement>();
  const width = Math.max(size.width, 320);
  const height = Math.max(size.height, 200);
  const innerWidth = Math.max(1, width - MARGIN.left - MARGIN.right);
  const numBins = Math.max(60, Math.min(400, Math.round(innerWidth / 4)));

  const { data, loading } = useResourceBlocking(what, dataRange.startTime, dataRange.endTime, numBins);

  // Few tasks → per-task gantt; many → the density area.
  const showGantt = !!data && data.total > 0 && data.total <= GANTT_THRESHOLD;
  const { tasks } = useResourceTasks(what, dataRange.startTime, dataRange.endTime, showGantt);

  // Color the gantt like the component/task views: cool task bars + warm milestone
  // waves (separate palettes), with this resource's blocking reason highlighted.
  const resourceKey = taskColorKey({ kind: HW_RESOURCE_KIND, what });
  const { taskColorMap, milestoneColorMap } = useMemo(() => {
    const taskKeys = tasks.map((t) => taskColorKey(t));
    const reasonKeys: string[] = [];
    for (const t of tasks) for (const step of milestonesOf(t.steps)) reasonKeys.push(taskColorKey(step));
    return {
      taskColorMap: buildColorMapFromKeys(taskKeys, "task"),
      milestoneColorMap: buildColorMapFromKeys(reasonKeys, "milestone"),
    };
  }, [tasks]);

  // Pan/zoom: the range stays local for smooth interaction and drives the data
  // fetch (debounced). Refs keep the wheel listener reading the latest values.
  const containerRef = useRef<HTMLDivElement | null>(null);
  const rangeRef = useRef(viewRange);
  rangeRef.current = viewRange;
  const widthRef = useRef(innerWidth);
  widthRef.current = innerWidth;
  const dragRef = useRef<{ x: number; range: TimeRange } | null>(null);
  const didDragRef = useRef(false);

  const applyRange = useCallback((next: TimeRange) => {
    setUserZoomed(true);
    setViewRange(sanitize(next.startTime, next.endTime));
  }, []);

  const zoomBy = useCallback(
    (factor: number) => {
      const r = rangeRef.current;
      const center = (r.startTime + r.endTime) / 2;
      const half = ((r.endTime - r.startTime) / 2) * factor;
      applyRange({ startTime: center - half, endTime: center + half });
    },
    [applyRange],
  );

  // ⌘/Ctrl+scroll zooms (anchored at the cursor); a plain scroll is left alone.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      const r = rangeRef.current;
      const rect = el.getBoundingClientRect();
      const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left - MARGIN.left) / widthRef.current));
      const dur = r.endTime - r.startTime;
      const scale = Math.pow(1.0015, event.deltaY);
      const anchor = r.startTime + dur * ratio;
      applyRange({ startTime: anchor - (anchor - r.startTime) * scale, endTime: anchor + (r.endTime - anchor) * scale });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, [applyRange]);

  const onPointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    dragRef.current = { x: event.clientX, range: rangeRef.current };
    didDragRef.current = false;
  };
  const onPointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag) return;
    const dx = event.clientX - drag.x;
    if (Math.abs(dx) > 2) didDragRef.current = true;
    const dur = drag.range.endTime - drag.range.startTime;
    const dt = (dur / widthRef.current) * dx;
    applyRange({ startTime: drag.range.startTime - dt, endTime: drag.range.endTime - dt });
  };
  const onPointerUp = () => {
    dragRef.current = null;
  };

  const startTime = viewRange.startTime;
  const endTime = viewRange.endTime;
  const xScale = useMemo(
    () => d3.scaleLinear().domain([startTime, endTime]).range([MARGIN.left, width - MARGIN.right]),
    [startTime, endTime, width],
  );

  const { areaPath, linePath, yScale, yTicks, hasData } = useMemo(() => {
    const bins = data?.bins ?? [];
    const maxV = Math.max(1, d3.max(bins) ?? 1);
    const y = d3.scaleLinear().domain([0, maxV]).nice().range([height - MARGIN.bottom, MARGIN.top]);
    const dStart = data?.start_time ?? startTime;
    const n = data?.num_bins ?? bins.length;
    const binW = n > 0 ? ((data?.end_time ?? endTime) - dStart) / n : 0;
    const pts = bins.map((v, b) => ({ t: dStart + (b + 0.5) * binW, v }));
    const area = d3.area<{ t: number; v: number }>().x((p) => xScale(p.t)).y0(y(0)).y1((p) => y(p.v)).curve(d3.curveMonotoneX);
    const line = d3.line<{ t: number; v: number }>().x((p) => xScale(p.t)).y((p) => y(p.v)).curve(d3.curveMonotoneX);
    return { areaPath: area(pts) ?? "", linePath: line(pts) ?? "", yScale: y, yTicks: y.ticks(5), hasData: bins.length > 0 };
  }, [data, xScale, startTime, endTime, height]);

  const panel = (
    <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4">
      <div>
        <SectionLabel>Hardware resource</SectionLabel>
        <div className="mt-2 break-all rounded-lg border bg-muted/30 p-3 font-mono text-xs">{what || "—"}</div>
      </div>
      {data ? (
        <dl className="flex flex-col gap-3 text-sm">
          <div className="flex flex-col gap-0.5">
            <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Tasks blocked (in view)</dt>
            <dd className="tabular-nums">
              {data.total.toLocaleString()}
              {data.total_all > data.total ? (
                <span className="ml-1 text-xs text-muted-foreground">of {data.total_all.toLocaleString()} total</span>
              ) : null}
            </dd>
          </div>
          {data.sample > 1 ? (
            <div className="flex flex-col gap-0.5">
              <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Estimate</dt>
              <dd className="text-xs text-muted-foreground">≈ from a 1-in-{data.sample} task sample</dd>
            </div>
          ) : null}
        </dl>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {showGantt
          ? "Each row is a task blocked on this resource; the highlighted span is the time it spent waiting for it (the dot is the release)."
          : "The shaded area is how many tasks are blocked waiting on this resource over time — its buildup and fall is where the contention forms and resolves."}
      </p>
    </div>
  );

  return (
    <TraceChartLayout panel={panel}>
      <div className="relative min-w-0 flex-1 bg-white">
        <div
          ref={(node) => {
            ref.current = node;
            containerRef.current = node;
          }}
          className="h-full w-full cursor-grab select-none active:cursor-grabbing"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerLeave={onPointerUp}
        >
          {!what ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No resource selected.</div>
          ) : loading && !data ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Computing…</div>
          ) : !hasData ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              No blocking recorded for this resource in range.
            </div>
          ) : (
            <svg width={width} height={height} className="block">
              {/* Y-axis count labels only in density mode. */}
              {!showGantt &&
                yTicks.map((tick) => (
                  <text
                    key={tick}
                    x={MARGIN.left - 6}
                    y={yScale(tick)}
                    textAnchor="end"
                    dominantBaseline="middle"
                    fontSize={AXIS_LABEL_FONT_SIZE}
                    fill={COLOR_AXIS_LABEL}
                  >
                    {tick}
                  </text>
                ))}
              <TimeTicks
                ticks={xScale.ticks(10)}
                xScale={xScale}
                gridTop={MARGIN.top}
                gridBottom={height - MARGIN.bottom}
                topLabelY={16}
                bottomLabelY={height - 8}
                tickMarks
              />
              <line x1={MARGIN.left} x2={width - MARGIN.right} y1={MARGIN.top} y2={MARGIN.top} stroke={COLOR_GRID} />
              <line x1={MARGIN.left} x2={width - MARGIN.right} y1={height - MARGIN.bottom} y2={height - MARGIN.bottom} stroke={COLOR_GRID} />

              {showGantt
                ? tasks.map((task, i) => {
                    // Each task is a bar (cool task color) with its milestone waves
                    // over it — the same primitives the component/task views use —
                    // and this resource's reason highlighted (others dim).
                    const availH = height - MARGIN.top - MARGIN.bottom;
                    const rowH = Math.min(26, Math.max(6, availH / Math.max(1, tasks.length)));
                    const barH = Math.max(4, Math.min(10, rowH - 6));
                    const barY = MARGIN.top + i * rowH + (rowH - barH) / 2;
                    const centerY = barY + barH / 2;
                    const left = MARGIN.left;
                    const right = width - MARGIN.right;
                    const x0 = Math.max(left, Math.min(right, xScale(task.start_time)));
                    const x1 = Math.max(left, Math.min(right, xScale(task.end_time)));
                    const steps = milestonesOf(task.steps).slice().sort((a, b) => a.time - b.time);
                    const blocked = blockedIntervals(task, what).reduce((sum, iv) => sum + (iv.hi - iv.lo), 0);
                    return (
                      <g key={task.id}>
                        <title>{`${task.kind} ${task.what} @ ${task.location} — blocked ${blocked.toLocaleString()} on ${what}`}</title>
                        <rect
                          x={x0}
                          y={barY}
                          width={Math.max(1, x1 - x0)}
                          height={barH}
                          fill={lookupColor(taskColorMap, task)}
                          stroke={COLOR_BAR_STROKE}
                          strokeWidth={0.75}
                          strokeOpacity={barStrokeOpacity({ selected: false, highlighted: false, hasHighlight: false })}
                          opacity={barOpacity({ selected: false, highlighted: false, hasHighlight: false, hasSelection: false })}
                        />
                        {steps.length > 0 && (
                          <MilestoneMarks
                            steps={steps}
                            taskStart={task.start_time}
                            xScale={xScale}
                            centerY={centerY}
                            colorMap={milestoneColorMap}
                            colorMode="kind-what"
                            selectedMilestone={null}
                            highlightedReason={resourceKey}
                            onSelect={() => {}}
                          />
                        )}
                      </g>
                    );
                  })
                : (
                  <>
                    <path d={areaPath} fill={FILL} fillOpacity={0.55} />
                    <path d={linePath} fill="none" stroke={STROKE} strokeWidth={1.5} />
                  </>
                )}
            </svg>
          )}
        </div>

        {/* Zoom controls + updating indicator. */}
        <div className="absolute right-3 top-3 flex items-center gap-2" onPointerDown={(e) => e.stopPropagation()}>
          {(dataPending || loading) && what ? (
            <span className="rounded bg-white/85 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground shadow-sm ring-1 ring-slate-200">
              Updating…
            </span>
          ) : null}
          <div className="inline-flex overflow-hidden rounded border bg-white/85 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm">
            <button type="button" className="px-1.5 py-1 text-muted-foreground hover:bg-muted" title="Zoom out" onClick={() => zoomBy(1.4)}>
              <Minus className="h-3.5 w-3.5" />
            </button>
            <button type="button" className="border-l px-1.5 py-1 text-muted-foreground hover:bg-muted" title="Zoom in" onClick={() => zoomBy(0.7)}>
              <Plus className="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      </div>
    </TraceChartLayout>
  );
}
