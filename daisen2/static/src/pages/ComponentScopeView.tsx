import * as d3 from "d3";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ChevronRight } from "lucide-react";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useTraceData } from "../hooks/useTraceData";
import type { Task } from "../types/task";
import { buildColorMapFromKeys, lookupColor, taskColorKey } from "../utils/taskColorCoder";
import { smartString } from "../utils/smartValue";
import {
  breadcrumbSegments,
  childPathFor,
  findNode,
  isLeafNode,
  type LocationNode,
} from "../utils/locationTree";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { cn } from "../lib/utils";

// Layout of the scoped timeline. Each direct child of the scope is one row: a
// label gutter on the left and a time track on the right. Leaf children draw
// their task bars; internal children (a port like "TLB.Top") render as a thin
// collapsed banner the user clicks to drill in — this keeps every drawn row
// single-kind (the "one location, one kind" invariant) and bounds the row count.
const LABEL_W = 220;
const ROW_H = 34;
const BAR_H = 16;
const AXIS_H = 24;
const PAD_RIGHT = 14;

function useContainerWidth<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) setWidth(entry.contentRect.width);
    });
    observer.observe(el);
    setWidth(el.clientWidth);
    return () => observer.disconnect();
  }, []);

  return { ref, width };
}

export default function ComponentScopeView({ root, scope }: { root: LocationNode; scope: string }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const { startTime: simStart, endTime: simEnd } = useSimulationRange();
  const startTime = Number(searchParams.get("starttime") ?? simStart);
  const endTime = Number(searchParams.get("endtime") ?? simEnd);

  const node = useMemo(() => findNode(root, scope), [root, scope]);
  const children = node?.children ?? [];

  const query = useMemo(() => ({ scope, startTime, endTime }), [scope, startTime, endTime]);
  const { tasks, loading } = useTraceData(query);

  const tasksByChild = useMemo(() => {
    const map = new Map<string, Task[]>();
    for (const task of tasks) {
      const childPath = childPathFor(scope, task.location);
      const list = map.get(childPath);
      if (list) list.push(task);
      else map.set(childPath, [task]);
    }
    return map;
  }, [tasks, scope]);

  const colorMap = useMemo(() => buildColorMapFromKeys(tasks.map(taskColorKey)), [tasks]);

  const { ref, width } = useContainerWidth<HTMLDivElement>();
  const trackX0 = LABEL_W;
  const trackX1 = Math.max(trackX0 + 1, width - PAD_RIGHT);
  const xScale = d3.scaleLinear().domain([startTime, endTime]).range([trackX0, trackX1]);
  const ticks = endTime > startTime ? xScale.ticks(8) : [];

  const navigateTo = (path: string) => {
    const params = new URLSearchParams(searchParams);
    params.set("name", path);
    params.set("starttime", String(startTime));
    params.set("endtime", String(endTime));
    setSearchParams(params);
  };

  const crumbs = breadcrumbSegments(scope);
  const svgHeight = AXIS_H + children.length * ROW_H;

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2">
        <nav className="flex items-center gap-1 text-sm">
          <Link to="/dashboard" className="text-muted-foreground hover:text-primary">
            Dashboard
          </Link>
          {crumbs.map((crumb, index) => {
            const isLast = index === crumbs.length - 1;
            return (
              <span key={crumb.path} className="flex items-center gap-1">
                <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                {isLast ? (
                  <span className="font-semibold">{crumb.label}</span>
                ) : (
                  <button className="text-muted-foreground hover:text-primary" onClick={() => navigateTo(crumb.path)}>
                    {crumb.label}
                  </button>
                )}
              </span>
            );
          })}
        </nav>

        <div className="ml-auto flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Drill into</span>
          <Select value="" onValueChange={navigateTo}>
            <SelectTrigger className="w-56">
              <SelectValue placeholder="next-level…" />
            </SelectTrigger>
            <SelectContent>
              {children.map((child) => (
                <SelectItem key={child.path} value={child.path}>
                  {child.name}
                  {isLeafNode(child) ? "" : " ›"}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div ref={ref} className="relative flex-1 overflow-auto">
        {loading && (
          <div className="absolute right-3 top-2 rounded bg-muted px-2 py-0.5 text-xs text-muted-foreground">
            loading…
          </div>
        )}
        <svg width={Math.max(1, width)} height={svgHeight} className="block">
          {ticks.map((tick) => (
            <g key={tick}>
              <line x1={xScale(tick)} x2={xScale(tick)} y1={AXIS_H} y2={svgHeight} stroke="#000" strokeDasharray="3,3" opacity={0.12} />
              <text x={xScale(tick)} y={AXIS_H - 8} textAnchor="middle" className="fill-muted-foreground text-[10px]">
                {smartString(tick)}
              </text>
            </g>
          ))}

          {children.map((child, index) => {
            const y0 = AXIS_H + index * ROW_H;
            const rowTasks = tasksByChild.get(child.path) ?? [];
            const leaf = isLeafNode(child);
            const barY = y0 + (ROW_H - BAR_H) / 2;

            return (
              <g key={child.path}>
                <line x1={0} x2={Math.max(1, width)} y1={y0} y2={y0} stroke="#000" opacity={0.06} />

                <text
                  x={12}
                  y={y0 + ROW_H / 2 + 4}
                  className="cursor-pointer fill-foreground text-xs hover:fill-primary"
                  onClick={() => navigateTo(child.path)}
                >
                  {child.name}
                  {!leaf && " ›"}
                </text>
                <text x={LABEL_W - 12} y={y0 + ROW_H / 2 + 4} textAnchor="end" className="fill-muted-foreground text-[10px]">
                  {leaf ? child.children.length === 0 ? rowTasks[0]?.kind ?? "" : "" : `${rowTasks.length} tasks`}
                </text>

                {leaf ? (
                  rowTasks.map((task) => {
                    const x = xScale(task.start_time);
                    const w = Math.max(1, xScale(task.end_time) - x);
                    return (
                      <rect
                        key={String(task.id)}
                        x={x}
                        y={barY}
                        width={w}
                        height={BAR_H}
                        fill={lookupColor(colorMap, task)}
                        stroke="#000"
                        strokeOpacity={0.2}
                        className="cursor-pointer"
                        onClick={() => navigateTo(child.path)}
                      >
                        <title>
                          {task.kind} - {task.what}
                          {"\n"}
                          {child.path}
                          {"\n"}
                          {smartString(task.start_time)} to {smartString(task.end_time)}
                        </title>
                      </rect>
                    );
                  })
                ) : (
                  <rect
                    x={trackX0}
                    y={barY}
                    width={Math.max(1, trackX1 - trackX0)}
                    height={BAR_H}
                    rx={3}
                    className={cn("cursor-pointer fill-muted stroke-border")}
                    strokeOpacity={0.6}
                    onClick={() => navigateTo(child.path)}
                  >
                    <title>Click to expand {child.path}</title>
                  </rect>
                )}
              </g>
            );
          })}
        </svg>
      </div>
    </div>
  );
}
