import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { RotateCcw, X, ChevronRight, ChevronDown, Search } from "lucide-react";
import DashboardWidget from "../components/DashboardWidget";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { useComponentNames } from "../hooks/useComponentNames";
import { useSegments } from "../hooks/useSegments";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useRenderReady } from "../hooks/useRenderReady";
import { parseView, mergeParams, DASHBOARD_DEFAULTS } from "../utils/viewState.mjs";
import { buildLocationTree, findNode, leafCount, breadcrumbSegments, type LocationNode } from "../utils/locationTree";
import { cn } from "../lib/utils";

const AXIS_OPTIONS = [
  { value: "ReqInCount", label: "Incoming Request Rate" },
  { value: "ReqCompleteCount", label: "Request Complete Rate" },
  { value: "AvgLatency", label: "Average Request Latency" },
  { value: "ConcurrentTask", label: "Number Concurrent Task" },
  { value: "RequestBufferPressure", label: "Request Buffer Pressure" },
  { value: "ResponseBufferPressure", label: "Response Buffer Pressure" },
  { value: "PendingReqOut", label: "Pending Request Out" },
  { value: "-", label: " - " },
];

// Resolve a URL axis param to a known metric key. Accepts the metric key or its
// human-readable label (shared/agent-generated links sometimes carry the label),
// and falls back to `fallback` for anything unrecognized so the chart shows the
// default metric instead of rendering empty.
function resolveAxis(raw: string | undefined, fallback: string): string {
  if (!raw) return fallback;
  const match = AXIS_OPTIONS.find(
    (option) => option.value === raw || option.label.trim() === raw.trim(),
  );
  return match ? match.value : fallback;
}

const DATA_RANGE_DEBOUNCE_MS = 1000;

interface TimeRange {
  startTime: number;
  endTime: number;
}

function useElementSize<T extends HTMLElement>() {
  const [size, setSize] = useState({ width: 1200, height: 720 });
  const observerRef = useRef<ResizeObserver | null>(null);

  // A callback ref so the observer (re)attaches whenever the measured node mounts.
  // The grid only renders after the components finish loading, so a mount-time
  // effect would run while the node doesn't exist yet and never observe it —
  // leaving `size` stuck at the default and the charts narrower than their cards.
  const ref = useCallback((node: T | null) => {
    observerRef.current?.disconnect();
    if (!node) return;
    const observer = new ResizeObserver(([entry]) => {
      setSize({ width: entry.contentRect.width, height: entry.contentRect.height });
    });
    observer.observe(node);
    observerRef.current = observer;
  }, []);

  return { ref, size };
}

function useDebouncedValue<T>(value: T, delayMs: number) {
  const [debouncedValue, setDebouncedValue] = useState(value);
  useEffect(() => {
    const timeout = window.setTimeout(() => setDebouncedValue(value), delayMs);
    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);
  return debouncedValue;
}

// keepForSearch returns the set of node paths to show when filtering the tree: a
// node is kept if its path matches OR it has a descendant that does, so the path
// down to every match stays navigable.
function keepForSearch(root: LocationNode, search: string): Set<string> | null {
  if (!search) return null;
  const q = search.toLowerCase();
  const keep = new Set<string>();
  const walk = (node: LocationNode): boolean => {
    let any = node.path.toLowerCase().includes(q);
    for (const child of node.children) {
      if (walk(child)) any = true;
    }
    if (any && node.path) keep.add(node.path);
    return any;
  };
  for (const child of root.children) walk(child);
  return keep;
}

// flatMatches lists the nodes whose own path matches the search (used to drive the
// grid in search mode: jump straight to the matching components' charts).
function flatMatches(root: LocationNode, search: string): string[] {
  const q = search.toLowerCase();
  const out: string[] = [];
  const walk = (node: LocationNode) => {
    if (node.path && node.path.toLowerCase().includes(q)) out.push(node.path);
    node.children.forEach(walk);
  };
  root.children.forEach(walk);
  return out;
}

// DashboardTree is the sidebar navigator: the location hierarchy, collapsed by
// default. The chevron expands/collapses a branch in place; clicking the name
// scopes the grid to that node's children (and the parent keeps the path to the
// current scope, plus any search matches, expanded).
function DashboardTree({
  nodes,
  scope,
  expanded,
  onScope,
  onToggle,
  depth,
  keep,
}: {
  nodes: LocationNode[];
  scope: string;
  expanded: Set<string>;
  onScope: (path: string) => void;
  onToggle: (path: string) => void;
  depth: number;
  keep: Set<string> | null;
}) {
  const visible = keep ? nodes.filter((n) => keep.has(n.path)) : nodes;
  if (visible.length === 0) return null;
  return (
    <ul className="space-y-0.5">
      {visible.map((node) => {
        const isBranch = node.children.length > 0;
        const isCurrent = node.path === scope;
        const open = isBranch && expanded.has(node.path);
        return (
          <li key={node.path}>
            <div
              className={cn("flex items-center rounded", isCurrent && "bg-primary/10")}
              style={{ paddingLeft: `${depth * 14}px` }}
            >
              {isBranch ? (
                <button
                  type="button"
                  className="flex h-6 w-5 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-primary"
                  onClick={() => onToggle(node.path)}
                  aria-label={open ? `Collapse ${node.name}` : `Expand ${node.name}`}
                >
                  {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                </button>
              ) : (
                <span className="flex h-6 w-5 shrink-0 items-center justify-center" aria-hidden="true">
                  <span className="h-1 w-1 rounded-full bg-muted-foreground/40" />
                </span>
              )}
              <button
                type="button"
                onClick={() => onScope(node.path)}
                title={node.path}
                className={cn(
                  "min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-xs transition-colors hover:text-primary",
                  isCurrent ? "font-medium text-primary" : isBranch ? "text-foreground" : "text-muted-foreground",
                )}
              >
                {node.name}
              </button>
            </div>
            {isBranch && open && (
              <DashboardTree nodes={node.children} scope={scope} expanded={expanded} onScope={onScope} onToggle={onToggle} depth={depth + 1} keep={keep} />
            )}
          </li>
        );
      })}
    </ul>
  );
}

export default function DashboardPage() {
  const { names, loading, error } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segmentsData } = useSegments();
  const [searchParams, setSearchParams] = useSearchParams();
  // The range string we last wrote to the URL, so the adopt-from-URL effect can
  // tell our own debounced writes from external navigation.
  const lastWrittenRangeRef = useRef<string | null>(null);
  // Whether there is a deliberate, non-sim range to reflect in the URL (set by a
  // user zoom/pan or an adopted external range; cleared by Reset Zoom).
  const userRangeRef = useRef(false);

  // The URL is the source of truth for the discrete view fields.
  const view = parseView("/dashboard", searchParams);
  const filter = view.filter ?? "";
  const scope = view.scope ?? "";
  const primaryAxis = resolveAxis(view.primary, DASHBOARD_DEFAULTS.primary);
  const secondaryAxis = resolveAxis(view.secondary, DASHBOARD_DEFAULTS.secondary);
  const widget = view.widget ?? "";

  const patchView = (patch: Record<string, string | number | undefined>) => {
    setSearchParams((prev) => mergeParams("/dashboard", prev, patch), { replace: true });
  };

  // A user zoom/pan is a deliberate range; remember that so the mirror writes it.
  const handleRangeChange = (range: TimeRange) => {
    userRangeRef.current = true;
    setViewRange(range);
  };

  // The time range stays local for smooth zoom/pan; it is seeded from the URL at
  // mount and mirrored back (debounced) below.
  const mountView = useRef(
    parseView("/dashboard", new URLSearchParams(window.location.search)),
  ).current;
  const urlHadRange = mountView.startTime !== undefined && mountView.endTime !== undefined;
  const [viewRange, setViewRange] = useState<TimeRange>(
    urlHadRange
      ? { startTime: mountView.startTime as number, endTime: mountView.endTime as number }
      : { startTime, endTime },
  );

  // Follow the simulation range only when the URL did not pin an explicit range.
  useEffect(() => {
    if (!urlHadRange) setViewRange({ startTime, endTime });
  }, [startTime, endTime, urlHadRange]);

  const dataRange = useDebouncedValue(viewRange, DATA_RANGE_DEBOUNCE_MS);
  const dataPending =
    viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;

  // Count the debounced data-range update as in-flight render work, so the
  // render-ready signal does not fire during the debounce window.
  useRenderReady(dataPending);

  // Mirror the (debounced) range into the URL, omitting it when it equals the
  // simulation range so a fresh dashboard URL stays "/dashboard".
  useEffect(() => {
    const atSim =
      !userRangeRef.current ||
      (viewRange.startTime === startTime && viewRange.endTime === endTime);
    lastWrittenRangeRef.current = atSim ? "" : `${dataRange.startTime}|${dataRange.endTime}`;
    setSearchParams(
      (prev) => {
        const next = mergeParams("/dashboard", prev, {
          startTime: atSim ? undefined : dataRange.startTime,
          endTime: atSim ? undefined : dataRange.endTime,
        });
        return next.toString() === prev.toString() ? prev : next;
      },
      { replace: true },
    );
  }, [
    dataRange.startTime,
    dataRange.endTime,
    viewRange.startTime,
    viewRange.endTime,
    startTime,
    endTime,
    setSearchParams,
  ]);

  // Adopt an externally-set range (shared/tour links or back/forward) without
  // reverting an in-progress local zoom — guarded by the range we last wrote.
  useEffect(() => {
    if (view.startTime === undefined || view.endTime === undefined) return;
    const key = `${view.startTime}|${view.endTime}`;
    if (key === lastWrittenRangeRef.current) return;
    lastWrittenRangeRef.current = key;
    userRangeRef.current = true;
    const next = { startTime: view.startTime, endTime: view.endTime };
    setViewRange((cur) => (cur.startTime === next.startTime && cur.endTime === next.endTime ? cur : next));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams]);

  const root = useMemo(() => buildLocationTree(names), [names]);
  const keep = useMemo(() => keepForSearch(root, filter), [root, filter]);
  const crumbs = useMemo(() => breadcrumbSegments(scope), [scope]);

  // What the grid shows: when searching, the matching components (flat); otherwise
  // the current scope's children — the top-level components at the root, the
  // facets one level down, and so on. A leaf scope shows its own chart.
  const gridNames = useMemo(() => {
    if (filter) return flatMatches(root, filter);
    const node = findNode(root, scope) ?? root;
    if (node.children.length > 0) return node.children.map((c) => c.path);
    return scope ? [scope] : [];
  }, [root, scope, filter]);

  const widgetNode = widget ? findNode(root, widget) : null;
  const widgetAggregated = !!widgetNode && widgetNode.children.length > 0;

  // The sidebar tree is collapsed by default; this tracks the branches the user
  // expanded with the chevrons.
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(() => new Set());
  const toggleNode = (path: string) =>
    setExpandedNodes((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  // What renders expanded: while searching, the matched subtree; otherwise the
  // user's expansions plus the path down to the current scope, so drilling into a
  // component reveals where you are.
  const effectiveExpanded = useMemo(() => {
    if (keep) return keep;
    const open = new Set(expandedNodes);
    for (const crumb of breadcrumbSegments(scope)) open.add(crumb.path);
    return open;
  }, [keep, expandedNodes, scope]);

  const { ref, size } = useElementSize<HTMLDivElement>();
  // Fewer, larger charts: 3 across on a wide screen, 2 on a medium one, 1 when
  // narrow — so each figure has room to breathe.
  const columns = size.width >= 1400 ? 3 : size.width >= 720 ? 2 : 1;
  const rows = Math.max(1, Math.ceil(gridNames.length / columns));
  // The grid has a 5px gap between cards (no outer gap), so a card is the area
  // minus the (columns-1) inner gaps, split evenly.
  const widgetWidth = Math.max(180, Math.floor((size.width - (columns - 1) * 5) / columns));
  const widgetHeight = Math.min(260, Math.max(160, Math.floor((size.height - (rows + 1) * 5) / Math.min(rows, 4))));

  const singleWidget = widget !== "";

  const axisSelect = (
    label: string,
    dot: string,
    value: string,
    onChange: (v: string) => void,
  ) => (
    <div className="flex min-w-64 items-center gap-2">
      <span className="flex items-center gap-1 text-sm font-medium">
        <span className="h-2.5 w-2.5 rounded-full" style={{ background: dot }} />
        {label}
      </span>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="w-52">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {AXIS_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <form
        className="flex min-h-12 flex-wrap items-center gap-3 border-b bg-white px-4 py-2"
        onSubmit={(event) => event.preventDefault()}
      >
        <Button
          type="button"
          onClick={() => {
            userRangeRef.current = false;
            setViewRange({ startTime, endTime });
          }}
        >
          <RotateCcw />
          Reset Zoom
        </Button>
        {singleWidget ? (
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-muted-foreground">Focused</span>
            <span className="max-w-72 truncate text-sm font-medium" title={widget}>
              {widget}
            </span>
            <Button type="button" variant="outline" size="sm" onClick={() => patchView({ widget: undefined })}>
              <X />
              Show all
            </Button>
          </div>
        ) : (
          // Breadcrumb of the current scope; click an ancestor to collapse back up.
          <nav className="flex min-w-0 flex-1 flex-wrap items-center gap-x-1 gap-y-0.5 text-sm">
            <button
              type="button"
              className={cn("rounded px-1 hover:text-primary", scope === "" ? "font-semibold" : "text-muted-foreground")}
              onClick={() => patchView({ scope: undefined, filter: undefined })}
            >
              All components
            </button>
            {crumbs.map((crumb, index) => (
              <span key={crumb.path} className="flex items-center gap-1">
                <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <button
                  type="button"
                  className={cn("truncate rounded px-1 hover:text-primary", index === crumbs.length - 1 ? "font-semibold" : "text-muted-foreground")}
                  onClick={() => patchView({ scope: crumb.path, filter: undefined })}
                >
                  {crumb.label}
                </button>
              </span>
            ))}
          </nav>
        )}
        {axisSelect("Primary Y-Axis", "#d7191c", primaryAxis, (value) => patchView({ primary: value }))}
        {axisSelect("Secondary Y-Axis", "#2c7bb6", secondaryAxis, (value) => patchView({ secondary: value }))}
      </form>

      {loading ? (
        <div className="flex flex-1 items-center justify-center text-muted-foreground">Loading components...</div>
      ) : error ? (
        <div className="flex flex-1 items-center justify-center text-destructive">{error}</div>
      ) : singleWidget ? (
        // Same callback ref as the grid: only one of the two is mounted at a time,
        // so the observer follows whichever is on screen. Without this the expanded
        // widget would inherit the grid's width (full width minus the sidebar) and
        // leave a sidebar-wide gap on the right. contentRect already excludes the
        // p-[5px], so the widget fills it exactly.
        <div ref={ref} className="min-h-0 flex-1 overflow-hidden bg-white p-[5px]">
          <DashboardWidget
            key={widget}
            name={widget}
            width={Math.max(180, size.width)}
            height={Math.max(120, size.height)}
            startTime={viewRange.startTime}
            endTime={viewRange.endTime}
            dataStartTime={dataRange.startTime}
            dataEndTime={dataRange.endTime}
            dataPending={dataPending}
            primaryAxis={primaryAxis}
            secondaryAxis={secondaryAxis}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            onTimeRangeChange={handleRangeChange}
            aggregated={widgetAggregated}
            facetCount={widgetAggregated && widgetNode ? leafCount(widgetNode) : undefined}
          />
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
          {/* Sidebar: the location tree + search. Click a node to scope the grid. */}
          <aside className="flex w-60 shrink-0 flex-col border-r bg-white">
            <div className="border-b p-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  className="h-8 pl-7 text-xs"
                  value={filter}
                  placeholder="Search components"
                  onChange={(event) => patchView({ filter: event.target.value || undefined })}
                />
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-auto p-1">
              <button
                type="button"
                className={cn("mb-0.5 flex w-full items-center rounded px-1.5 py-1 text-left text-xs hover:bg-muted", scope === "" && !filter ? "bg-primary/10 font-medium text-primary" : "text-foreground")}
                onClick={() => patchView({ scope: undefined, filter: undefined })}
              >
                All components
              </button>
              <DashboardTree
                nodes={root.children}
                scope={scope}
                expanded={effectiveExpanded}
                onScope={(path) => patchView({ scope: path || undefined, filter: undefined })}
                onToggle={toggleNode}
                depth={0}
                keep={keep}
              />
            </div>
          </aside>

          {/* Grid of the scope's children (or search matches), each chart aggregating
              its own subtree. */}
          <div ref={ref} className="min-h-0 flex-1 overflow-auto bg-white p-[5px]">
            {gridNames.length === 0 ? (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                {filter ? "No components match the search." : "No components to show."}
              </div>
            ) : (
              <div
                className="daisen-dashboard-grid"
                style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: `${widgetHeight}px` }}
              >
                {gridNames.map((name) => {
                  const node = findNode(root, name);
                  const aggregated = !!node && node.children.length > 0;
                  return (
                    <DashboardWidget
                      key={name}
                      name={name}
                      width={widgetWidth}
                      height={widgetHeight}
                      startTime={viewRange.startTime}
                      endTime={viewRange.endTime}
                      dataStartTime={dataRange.startTime}
                      dataEndTime={dataRange.endTime}
                      dataPending={dataPending}
                      primaryAxis={primaryAxis}
                      secondaryAxis={secondaryAxis}
                      segments={segmentsData?.segments ?? []}
                      segmentsEnabled={segmentsData?.enabled ?? false}
                      onTimeRangeChange={handleRangeChange}
                      onFocus={(focused) => patchView({ widget: focused })}
                      aggregated={aggregated}
                      facetCount={aggregated && node ? leafCount(node) : undefined}
                    />
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
