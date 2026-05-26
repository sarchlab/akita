import { useCallback, useEffect, useRef, useState, type PointerEvent } from "react";
import { Activity, RotateCcw, ZoomIn, ZoomOut } from "lucide-react";
import { Button } from "../components/ui/button";

interface ResourceResponse {
  cpu_percent: number;
  memory_size: number;
}

interface ResourcePoint extends ResourceResponse {
  timestamp: number;
}

interface MinuteResourcePoint extends ResourcePoint {
  samples: number;
}

interface ResourceHistory {
  seconds: ResourcePoint[];
  minutes: MinuteResourcePoint[];
}

interface ProfileSummary {
  samples: number;
  locations: number;
  functions: number;
  topFunctions: ProfileFunctionStat[];
  callGraph: ProfileCallGraph;
}

interface ProfileFunctionStat {
  name: string;
  value: number;
}

interface ProfileCallGraph {
  nodes: ProfileCallGraphNode[];
  edges: ProfileCallGraphEdge[];
}

interface ProfileCallGraphNode {
  id: string;
  label: string;
  value: number;
  depth: number;
}

interface ProfileCallGraphEdge {
  id: string;
  from: string;
  to: string;
  value: number;
}

const RESOURCE_SAMPLE_INTERVAL_MS = 1000;
const MAX_SECOND_SAMPLES = 60;
const MAX_MINUTE_SAMPLES = 60;
const CALL_GRAPH_MIN_SCALE = 0.35;
const CALL_GRAPH_MAX_SCALE = 4;
const CALL_GRAPH_ZOOM_STEP = 1.2;

interface CallGraphViewport {
  scale: number;
  x: number;
  y: number;
}

const INITIAL_CALL_GRAPH_VIEWPORT: CallGraphViewport = { scale: 1, x: 0, y: 0 };

function formatBytes(bytes: number | null | undefined) {
  if (typeof bytes !== "number" || !Number.isFinite(bytes)) {
    return "-";
  }

  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = bytes;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const digits = value >= 10 || unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unitIndex]}`;
}

function useResourceUsage() {
  const [history, setHistory] = useState<ResourceHistory>({ seconds: [], minutes: [] });

  const refresh = useCallback(() => {
    fetch("/api/resource")
      .then((response) => (response.ok ? response.json() : null))
      .then((json: unknown) => {
        if (!json || typeof json !== "object") {
          return;
        }

        const resource = json as Partial<ResourceResponse>;
        const nextResources = {
          cpu_percent: typeof resource.cpu_percent === "number" ? resource.cpu_percent : 0,
          memory_size: typeof resource.memory_size === "number" ? resource.memory_size : 0,
        };
        const nextPoint = { ...nextResources, timestamp: Date.now() };

        setHistory((previous) => {
          const seconds = [...previous.seconds, nextPoint].slice(-MAX_SECOND_SAMPLES);
          const minuteTimestamp = Math.floor(nextPoint.timestamp / 60000) * 60000;
          const lastMinute = previous.minutes[previous.minutes.length - 1];
          let minutes: MinuteResourcePoint[];

          if (lastMinute?.timestamp === minuteTimestamp) {
            const samples = lastMinute.samples + 1;
            const updatedMinute = {
              timestamp: minuteTimestamp,
              samples,
              cpu_percent:
                (lastMinute.cpu_percent * lastMinute.samples + nextPoint.cpu_percent) /
                samples,
              memory_size:
                (lastMinute.memory_size * lastMinute.samples + nextPoint.memory_size) /
                samples,
            };

            minutes = [...previous.minutes.slice(0, -1), updatedMinute];
          } else {
            minutes = [
              ...previous.minutes,
              { ...nextResources, timestamp: minuteTimestamp, samples: 1 },
            ];
          }

          return { seconds, minutes: minutes.slice(-MAX_MINUTE_SAMPLES) };
        });
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, RESOURCE_SAMPLE_INTERVAL_MS);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { history };
}

function getArray(value: Record<string, unknown>, lower: string, upper: string): unknown[] {
  const array = Array.isArray(value[lower]) ? value[lower] : Array.isArray(value[upper]) ? value[upper] : [];
  return array;
}

function getObject(value: Record<string, unknown>, lower: string, upper: string): Record<string, unknown> | null {
  const candidate = value[lower] ?? value[upper];
  return candidate && typeof candidate === "object" ? (candidate as Record<string, unknown>) : null;
}

function getNumberArray(value: Record<string, unknown>, lower: string, upper: string): number[] {
  return getArray(value, lower, upper).filter((item): item is number => typeof item === "number");
}

function profileLocationName(location: unknown) {
  if (!location || typeof location !== "object") {
    return "unknown";
  }

  const locationObject = location as Record<string, unknown>;
  const lines = getArray(locationObject, "line", "Line");
  const line = lines[0];
  if (line && typeof line === "object") {
    const lineObject = line as Record<string, unknown>;
    const fn = getObject(lineObject, "function", "Function");
    if (fn) {
      const name = fn.name ?? fn.Name ?? fn.systemName ?? fn.SystemName;
      if (typeof name === "string" && name) {
        return name;
      }
    }
  }

  const address = locationObject.address ?? locationObject.Address;
  if (typeof address === "number") {
    return `0x${address.toString(16)}`;
  }

  return "unknown";
}

function profileStackNames(sample: unknown) {
  if (!sample || typeof sample !== "object") {
    return [];
  }

  const sampleObject = sample as Record<string, unknown>;
  const locations = getArray(sampleObject, "location", "Location");

  return locations.map(profileLocationName).filter((name) => name !== "unknown");
}

function profileFunctionName(sample: unknown) {
  return profileStackNames(sample)[0] ?? "unknown";
}

function sampleWeight(sample: Record<string, unknown>) {
  const values = getNumberArray(sample, "value", "Value");
  return Math.max(1, ...values.map((value) => Math.abs(value)));
}

function buildCallGraph(samples: unknown[]): ProfileCallGraph {
  const nodeTotals = new Map<string, { value: number; depthTotal: number; count: number }>();
  const edgeTotals = new Map<string, ProfileCallGraphEdge>();

  samples.forEach((item) => {
    if (!item || typeof item !== "object") {
      return;
    }

    const sample = item as Record<string, unknown>;
    const weight = sampleWeight(sample);
    const leafFirstStack = profileStackNames(sample);
    const callerFirstStack = [...leafFirstStack].reverse().filter((name, index, stack) => {
      return index === 0 || name !== stack[index - 1];
    });

    callerFirstStack.forEach((name, depth) => {
      const node = nodeTotals.get(name) ?? { value: 0, depthTotal: 0, count: 0 };
      node.value += weight;
      node.depthTotal += depth;
      node.count += 1;
      nodeTotals.set(name, node);
    });

    for (let i = 0; i < callerFirstStack.length - 1; i += 1) {
      const from = callerFirstStack[i];
      const to = callerFirstStack[i + 1];
      const id = `${from}->${to}`;
      const edge = edgeTotals.get(id) ?? { id, from, to, value: 0 };
      edge.value += weight;
      edgeTotals.set(id, edge);
    }
  });

  const selectedNodeIDs = new Set(
    [...nodeTotals.entries()]
      .sort((a, b) => b[1].value - a[1].value)
      .slice(0, 24)
      .map(([id]) => id),
  );

  const edges = [...edgeTotals.values()]
    .filter((edge) => selectedNodeIDs.has(edge.from) && selectedNodeIDs.has(edge.to))
    .sort((a, b) => b.value - a.value)
    .slice(0, 40);

  edges.forEach((edge) => {
    selectedNodeIDs.add(edge.from);
    selectedNodeIDs.add(edge.to);
  });

  const nodes = [...selectedNodeIDs]
    .map((id) => {
      const node = nodeTotals.get(id);
      return {
        id,
        label: id,
        value: node?.value ?? 0,
        depth: node && node.count ? Math.round(node.depthTotal / node.count) : 0,
      };
    })
    .sort((a, b) => a.depth - b.depth || b.value - a.value);

  return { nodes, edges };
}

function summarizeProfile(profile: unknown): ProfileSummary {
  if (!profile || typeof profile !== "object") {
    return { samples: 0, locations: 0, functions: 0, topFunctions: [], callGraph: { nodes: [], edges: [] } };
  }

  const p = profile as Record<string, unknown>;
  const sample = Array.isArray(p.sample) ? p.sample : Array.isArray(p.Sample) ? p.Sample : [];
  const location = Array.isArray(p.location) ? p.location : Array.isArray(p.Location) ? p.Location : [];
  const fn = Array.isArray(p.function) ? p.function : Array.isArray(p.Function) ? p.Function : [];
  const functionTotals = new Map<string, number>();

  sample.forEach((item) => {
    if (!item || typeof item !== "object") {
      return;
    }

    const sampleObject = item as Record<string, unknown>;
    const weight = sampleWeight(sampleObject);
    const name = profileFunctionName(item);

    functionTotals.set(name, (functionTotals.get(name) ?? 0) + weight);
  });

  const topFunctions = [...functionTotals.entries()]
    .map(([name, value]) => ({ name, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 8);

  return {
    samples: sample.length,
    locations: location.length,
    functions: fn.length,
    topFunctions,
    callGraph: buildCallGraph(sample),
  };
}

export default function ProfilingPage() {
  const { history } = useResourceUsage();
  const [profileSeconds, setProfileSeconds] = useState(1);
  const [profileStatus, setProfileStatus] = useState("");
  const [profileSummary, setProfileSummary] = useState<ProfileSummary | null>(null);
  const [isCapturing, setIsCapturing] = useState(false);

  const captureProfile = async () => {
    setIsCapturing(true);
    setProfileStatus(`Capturing ${profileSeconds}s CPU profile...`);
    try {
      const response = await fetch(`/api/profile?seconds=${profileSeconds}`);
      if (!response.ok) {
        throw new Error(`${response.status} ${response.statusText}`);
      }

      const profile = await response.json();
      setProfileSummary(summarizeProfile(profile));
      setProfileStatus("Profile captured");
    } catch (err) {
      setProfileStatus(err instanceof Error ? err.message : "Profile capture failed");
    } finally {
      setIsCapturing(false);
    }
  };

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <section className="grid gap-4 md:grid-cols-2">
          <div className="rounded border bg-white p-4">
            <div className="mb-2 text-sm font-semibold text-slate-950">Resource Usage</div>
            <ResourceTrendChart secondHistory={history.seconds} minuteHistory={history.minutes} />
          </div>

          <div className="rounded border bg-white p-4">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div className="text-sm font-semibold">Latest CPU Profile</div>
              <div className="flex flex-wrap items-center gap-2">
                <label className="text-xs text-muted-foreground" htmlFor="profile-seconds">
                  Seconds
                </label>
                <select
                  id="profile-seconds"
                  className="h-8 rounded border border-input bg-background px-2 text-sm"
                  value={profileSeconds}
                  onChange={(event) => setProfileSeconds(Number(event.target.value))}
                  disabled={isCapturing}
                >
                  {[1, 2, 5, 10, 30].map((seconds) => (
                    <option key={seconds} value={seconds}>
                      {seconds}
                    </option>
                  ))}
                </select>
                <Button type="button" size="sm" onClick={captureProfile} disabled={isCapturing}>
                  <Activity /> Capture CPU Profile
                </Button>
              </div>
            </div>
            {profileStatus ? <div className="mb-3 text-sm text-muted-foreground">{profileStatus}</div> : null}
            {profileSummary ? (
              <ProfileMetricBars summary={profileSummary} />
            ) : (
              <div className="text-sm text-muted-foreground">No profile captured yet.</div>
            )}
          </div>
        </section>

        <section className="rounded border bg-white p-4">
          <div className="mb-3 text-sm font-semibold">CPU Call Graph</div>
          {profileSummary ? (
            <CallGraph graph={profileSummary.callGraph} />
          ) : (
            <div className="text-sm text-muted-foreground">Capture a CPU profile to generate a call graph.</div>
          )}
        </section>

        <section className="rounded border bg-white p-4">
          <div className="mb-3 text-sm font-semibold">Top Functions</div>
          {profileSummary ? (
            <TopFunctionBars functions={profileSummary.topFunctions} />
          ) : (
            <div className="text-sm text-muted-foreground">Capture a CPU profile to populate function samples.</div>
          )}
        </section>
      </div>
    </div>
  );
}

function ProfileMetricBars({ summary }: { summary: ProfileSummary }) {
  const metrics = [
    { label: "Samples", value: summary.samples },
    { label: "Locations", value: summary.locations },
    { label: "Functions", value: summary.functions },
  ];
  const max = Math.max(1, ...metrics.map((metric) => metric.value));

  return (
    <div className="space-y-3">
      {metrics.map((metric) => (
        <div key={metric.label}>
          <div className="mb-1 flex items-center justify-between text-xs">
            <span className="font-medium">{metric.label}</span>
            <span className="font-mono text-muted-foreground">{metric.value}</span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-slate-200">
            <div className="h-full bg-sky-600" style={{ width: `${(metric.value / max) * 100}%` }} />
          </div>
        </div>
      ))}
    </div>
  );
}

function TopFunctionBars({ functions }: { functions: ProfileFunctionStat[] }) {
  const max = Math.max(1, ...functions.map((fn) => fn.value));

  if (!functions.length) {
    return <div className="text-sm text-muted-foreground">No function samples in the captured profile.</div>;
  }

  return (
    <div className="space-y-3">
      {functions.map((fn) => (
        <div key={fn.name}>
          <div className="mb-1 flex items-center justify-between gap-3 text-xs">
            <span className="min-w-0 truncate font-medium">{fn.name}</span>
            <span className="shrink-0 font-mono text-muted-foreground">{fn.value}</span>
          </div>
          <div className="h-2 overflow-hidden rounded-full bg-slate-200">
            <div className="h-full bg-amber-500" style={{ width: `${(fn.value / max) * 100}%` }} />
          </div>
        </div>
      ))}
    </div>
  );
}

function shortFunctionName(name: string) {
  const parts = name.split("/");
  const lastPath = parts[parts.length - 1] ?? name;
  if (lastPath.length <= 30) {
    return lastPath;
  }

  return `${lastPath.slice(0, 27)}...`;
}

function formatSampleCount(value: number) {
  const units = ["", "K", "M", "B", "T"];
  let scaled = Math.abs(value);
  let unitIndex = 0;

  while (scaled >= 1000 && unitIndex < units.length - 1) {
    scaled /= 1000;
    unitIndex += 1;
  }

  const digits = scaled >= 10 || unitIndex === 0 ? 0 : 1;
  const sign = value < 0 ? "-" : "";

  return `${sign}${scaled.toFixed(digits)}${units[unitIndex]}`;
}

function clampCallGraphScale(scale: number) {
  return Math.max(CALL_GRAPH_MIN_SCALE, Math.min(CALL_GRAPH_MAX_SCALE, scale));
}

function CallGraph({ graph }: { graph: ProfileCallGraph }) {
  const [viewport, setViewport] = useState<CallGraphViewport>(INITIAL_CALL_GRAPH_VIEWPORT);
  const [isPanning, setIsPanning] = useState(false);
  const dragStartRef = useRef<{ clientX: number; clientY: number } | null>(null);
  const wheelTargetRef = useRef<HTMLDivElement | null>(null);
  const wheelHandlerRef = useRef<((event: WheelEvent) => void) | null>(null);

  useEffect(() => {
    setViewport(INITIAL_CALL_GRAPH_VIEWPORT);
    setIsPanning(false);
    dragStartRef.current = null;
  }, [graph]);

  useEffect(() => {
    const wheelTarget = wheelTargetRef.current;
    if (!wheelTarget) {
      return;
    }

    const handleNativeWheel = (event: WheelEvent) => {
      wheelHandlerRef.current?.(event);
    };

    wheelTarget.addEventListener("wheel", handleNativeWheel, { passive: false });

    return () => {
      wheelTarget.removeEventListener("wheel", handleNativeWheel);
    };
  }, [graph]);

  if (!graph.nodes.length) {
    wheelHandlerRef.current = null;
    return <div className="text-sm text-muted-foreground">No call graph samples in the captured profile.</div>;
  }

  const left = 32;
  const top = 28;
  const nodeWidth = 220;
  const nodeHeight = 44;
  const rowGap = 58;
  const columnGap = 250;
  const maxVisibleDepth = Math.min(10, Math.max(0, ...graph.nodes.map((node) => node.depth)));
  const depthFor = (node: ProfileCallGraphNode) => Math.max(0, Math.min(maxVisibleDepth, node.depth));
  const grouped = new Map<number, ProfileCallGraphNode[]>();
  const nodesByID = new Map(graph.nodes.map((node) => [node.id, node]));
  const incomingNodeIDs = new Set(graph.edges.map((edge) => edge.to));
  const outgoing = new Map<string, ProfileCallGraphEdge[]>();

  graph.edges.forEach((edge) => {
    outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge]);
  });

  outgoing.forEach((edges, from) => {
    outgoing.set(from, [...edges].sort((a, b) => b.value - a.value));
  });

  const hotPathNodeIDs = new Set<string>();
  const hotPathEdgeIDs = new Set<string>();
  const roots = graph.nodes
    .filter((node) => !incomingNodeIDs.has(node.id))
    .sort((a, b) => a.depth - b.depth || b.value - a.value);
  let currentNode: ProfileCallGraphNode | undefined =
    roots[0] ?? [...graph.nodes].sort((a, b) => b.value - a.value)[0];

  while (currentNode && !hotPathNodeIDs.has(currentNode.id) && hotPathNodeIDs.size < 16) {
    hotPathNodeIDs.add(currentNode.id);

    const nextEdge = (outgoing.get(currentNode.id) ?? []).find((edge) => {
      return nodesByID.has(edge.to) && !hotPathNodeIDs.has(edge.to);
    });
    if (!nextEdge) {
      break;
    }

    hotPathEdgeIDs.add(nextEdge.id);
    currentNode = nodesByID.get(nextEdge.to);
  }

  graph.nodes.forEach((node) => {
    const depth = depthFor(node);
    grouped.set(depth, [...(grouped.get(depth) ?? []), node]);
  });

  const depths = [...grouped.keys()].sort((a, b) => a - b);
  const maxRows = Math.max(1, ...depths.map((depth) => grouped.get(depth)?.length ?? 0));
  const width = Math.max(900, left * 2 + Math.max(0, depths.length - 1) * columnGap + nodeWidth);
  const height = Math.max(320, top * 2 + Math.max(0, maxRows - 1) * rowGap + nodeHeight);
  const maxNodeValue = Math.max(1, ...graph.nodes.map((node) => node.value));
  const maxEdgeValue = Math.max(1, ...graph.edges.map((edge) => edge.value));
  const positions = new Map<string, { x: number; y: number }>();

  depths.forEach((depth, columnIndex) => {
    const nodes = [...(grouped.get(depth) ?? [])].sort((a, b) => {
      const hotPathSort = Number(hotPathNodeIDs.has(b.id)) - Number(hotPathNodeIDs.has(a.id));
      return hotPathSort || b.value - a.value;
    });

    nodes.forEach((node, rowIndex) => {
      positions.set(node.id, {
        x: left + columnIndex * columnGap,
        y: top + rowIndex * rowGap,
      });
    });
  });

  const visibleEdges = graph.edges
    .filter((edge) => positions.has(edge.from) && positions.has(edge.to))
    .sort((a, b) => a.value - b.value);
  const zoomAround = (anchorX: number, anchorY: number, factor: number) => {
    setViewport((previous) => {
      const scale = clampCallGraphScale(previous.scale * factor);
      const graphX = (anchorX - previous.x) / previous.scale;
      const graphY = (anchorY - previous.y) / previous.scale;

      return {
        scale,
        x: anchorX - graphX * scale,
        y: anchorY - graphY * scale,
      };
    });
  };
  wheelHandlerRef.current = (event: WheelEvent) => {
    event.preventDefault();
    event.stopPropagation();

    const wheelTarget = wheelTargetRef.current;
    if (!wheelTarget) {
      return;
    }

    const rect = wheelTarget.getBoundingClientRect();
    const anchorX = ((event.clientX - rect.left) / rect.width) * width;
    const anchorY = ((event.clientY - rect.top) / rect.height) * height;
    const factor = event.deltaY < 0 ? CALL_GRAPH_ZOOM_STEP : 1 / CALL_GRAPH_ZOOM_STEP;

    zoomAround(anchorX, anchorY, factor);
  };
  const handlePointerDown = (event: PointerEvent<SVGSVGElement>) => {
    if (event.button !== 0) {
      return;
    }

    event.currentTarget.setPointerCapture(event.pointerId);
    dragStartRef.current = { clientX: event.clientX, clientY: event.clientY };
    setIsPanning(true);
  };
  const handlePointerMove = (event: PointerEvent<SVGSVGElement>) => {
    if (!dragStartRef.current) {
      return;
    }

    const rect = event.currentTarget.getBoundingClientRect();
    const dx = ((event.clientX - dragStartRef.current.clientX) / rect.width) * width;
    const dy = ((event.clientY - dragStartRef.current.clientY) / rect.height) * height;

    dragStartRef.current = { clientX: event.clientX, clientY: event.clientY };
    setViewport((previous) => ({ ...previous, x: previous.x + dx, y: previous.y + dy }));
  };
  const finishPointerPan = (event: PointerEvent<SVGSVGElement>) => {
    dragStartRef.current = null;
    setIsPanning(false);

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };

  return (
    <div className="rounded border bg-slate-50 p-2">
      <div className="mb-2 flex items-center justify-end gap-1">
        <div className="mr-1 font-mono text-xs text-slate-700">{Math.round(viewport.scale * 100)}%</div>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7"
          title="Zoom out"
          aria-label="Zoom out call graph"
          onClick={() => zoomAround(width / 2, height / 2, 1 / CALL_GRAPH_ZOOM_STEP)}
        >
          <ZoomOut />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7"
          title="Zoom in"
          aria-label="Zoom in call graph"
          onClick={() => zoomAround(width / 2, height / 2, CALL_GRAPH_ZOOM_STEP)}
        >
          <ZoomIn />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7"
          title="Reset view"
          aria-label="Reset call graph view"
          onClick={() => setViewport(INITIAL_CALL_GRAPH_VIEWPORT)}
        >
          <RotateCcw />
        </Button>
      </div>
      <div ref={wheelTargetRef} className="h-[32rem] overscroll-contain overflow-hidden rounded border bg-white">
        <svg
          className={`h-full w-full select-none touch-none ${isPanning ? "cursor-grabbing" : "cursor-grab"}`}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="CPU profile call graph"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={finishPointerPan}
          onPointerCancel={finishPointerPan}
        >
          <defs>
            <marker id="call-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
              <path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b" />
            </marker>
          </defs>
          <g transform={`translate(${viewport.x} ${viewport.y}) scale(${viewport.scale})`}>
            {visibleEdges.map((edge) => {
              const from = positions.get(edge.from)!;
              const to = positions.get(edge.to)!;
              const startX = from.x + nodeWidth;
              const startY = from.y + nodeHeight / 2;
              const endX = to.x;
              const endY = to.y + nodeHeight / 2;
              const forward = endX > startX;
              const bend = forward ? Math.max(58, (endX - startX) / 2) : 44;
              const isHotPath = hotPathEdgeIDs.has(edge.id);
              const strokeWidth = isHotPath ? 2.5 + (edge.value / maxEdgeValue) * 2 : 0.9 + (edge.value / maxEdgeValue) * 2.5;
              const edgePath = forward
                ? `M ${startX} ${startY} C ${startX + bend} ${startY}, ${endX - bend} ${endY}, ${endX - 6} ${endY}`
                : `M ${startX} ${startY} C ${startX + bend} ${startY}, ${startX + bend} ${endY}, ${endX - 6} ${endY}`;

              return (
                <path
                  key={edge.id}
                  d={edgePath}
                  fill="none"
                  stroke={isHotPath ? "#475569" : "#94a3b8"}
                  strokeOpacity={isHotPath ? 0.78 : 0.16 + (edge.value / maxEdgeValue) * 0.42}
                  strokeWidth={strokeWidth}
                  markerEnd="url(#call-arrow)"
                >
                  <title>
                    {edge.from} {"->"} {edge.to}: {edge.value}
                  </title>
                </path>
              );
            })}
            {graph.nodes.map((node) => {
              const position = positions.get(node.id);
              if (!position) {
                return null;
              }

              const intensity = node.value / maxNodeValue;
              const isHotPath = hotPathNodeIDs.has(node.id);

              return (
                <g key={node.id} transform={`translate(${position.x} ${position.y})`}>
                  <rect
                    width={nodeWidth}
                    height={nodeHeight}
                    rx="6"
                    fill="#ffffff"
                    stroke={isHotPath || intensity > 0.66 ? "#0284c7" : "#cbd5e1"}
                    strokeWidth={isHotPath ? 2.5 : 1 + intensity * 1.5}
                  />
                  <rect width={Math.max(4, nodeWidth * intensity)} height="4" rx="2" fill="#0284c7" />
                  <text x="10" y="19" className="fill-slate-950 text-[12px] font-semibold">
                    {shortFunctionName(node.label)}
                  </text>
                  <text x="10" y="35" className="fill-slate-600 text-[11px]">
                    samples {formatSampleCount(node.value)}
                  </text>
                  <title>
                    {node.label}: {node.value}
                  </title>
                </g>
              );
            })}
          </g>
        </svg>
      </div>
    </div>
  );
}

interface MetricTrendPoint {
  timestamp: number;
  value: number;
  formattedValue: string;
  samples?: number;
}

interface ActiveTrendPoint extends MetricTrendPoint {
  leftPercent: number;
  topPercent: number;
}

function formatChartTime(timestamp: number, includeSeconds = true) {
  return new Date(timestamp).toLocaleTimeString([], {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    ...(includeSeconds ? { second: "2-digit" } : {}),
  });
}

function formatTooltipTime(timestamp: number) {
  return new Date(timestamp).toLocaleString([], {
    month: "short",
    day: "2-digit",
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function tickIndexesFor(points: MetricTrendPoint[], maxTicks = 6) {
  if (points.length <= 1) {
    return points.length ? [0] : [];
  }

  const tickCount = Math.min(maxTicks, points.length);
  return Array.from(
    new Set(
      Array.from({ length: tickCount }, (_, index) =>
        Math.round((index / (tickCount - 1)) * (points.length - 1)),
      ),
    ),
  );
}

function metricPoints(
  points: ResourcePoint[],
  valueFor: (point: ResourcePoint) => number,
  formatValue: (value: number) => string,
): MetricTrendPoint[] {
  return points.map((point) => ({
    timestamp: point.timestamp,
    value: valueFor(point),
    formattedValue: formatValue(valueFor(point)),
    samples: "samples" in point && typeof point.samples === "number" ? point.samples : undefined,
  }));
}

function ResourceTrendChart({
  secondHistory,
  minuteHistory,
}: {
  secondHistory: ResourcePoint[];
  minuteHistory: MinuteResourcePoint[];
}) {
  const fallback = { cpu_percent: 0, memory_size: 0, timestamp: Date.now() };
  const seconds = secondHistory.length ? secondHistory : [fallback];
  const minutes = minuteHistory.length ? minuteHistory : [{ ...fallback, samples: 1 }];

  return (
    <div className="grid gap-3">
      <MetricTrendFigure
        title="CPU"
        color="#0369a1"
        secondHistory={seconds}
        minuteHistory={minutes}
        minimumMax={100}
        valueFor={(point) => point.cpu_percent}
        formatValue={(value) => `${value.toFixed(1)}%`}
      />
      <MetricTrendFigure
        title="RSS"
        color="#b45309"
        secondHistory={seconds}
        minuteHistory={minutes}
        valueFor={(point) => point.memory_size}
        formatValue={formatBytes}
      />
    </div>
  );
}

function MetricTrendFigure({
  title,
  color,
  secondHistory,
  minuteHistory,
  minimumMax = 1,
  valueFor,
  formatValue,
}: {
  title: string;
  color: string;
  secondHistory: ResourcePoint[];
  minuteHistory: MinuteResourcePoint[];
  minimumMax?: number;
  valueFor: (point: ResourcePoint) => number;
  formatValue: (value: number) => string;
}) {
  const secondPoints = metricPoints(secondHistory, valueFor, formatValue);
  const minutePoints = metricPoints(minuteHistory, valueFor, formatValue);
  const latestPoint = secondPoints[secondPoints.length - 1] ?? minutePoints[minutePoints.length - 1];
  const maxValue = Math.max(
    minimumMax,
    ...secondPoints.map((point) => point.value),
    ...minutePoints.map((point) => point.value),
  );
  const chartMax = title === "CPU" ? maxValue : maxValue * 1.12;

  return (
    <section className="border-b border-slate-300 pb-3 last:border-b-0 last:pb-0">
      <div className="mb-1 flex items-center justify-between gap-3">
        <div className="inline-flex items-center gap-2 text-sm font-semibold text-slate-950">
          <span className="h-2 w-5 rounded-full" style={{ backgroundColor: color }} />
          {title}
        </div>
        <div className="font-mono text-sm font-semibold text-slate-800">{latestPoint?.formattedValue ?? "-"}</div>
      </div>
      <div className="grid gap-3 xl:grid-cols-2">
        <TrendSegmentChart
          title="Last minute"
          detail={`${secondPoints.length}/${MAX_SECOND_SAMPLES} per-second samples`}
          points={secondPoints}
          color={color}
          maxValue={chartMax}
          includeSeconds
        />
        <TrendSegmentChart
          title="Last 60 minutes"
          detail={`${minutePoints.length}/${MAX_MINUTE_SAMPLES} per-minute averages`}
          points={minutePoints}
          color={color}
          maxValue={chartMax}
        />
      </div>
    </section>
  );
}

function TrendSegmentChart({
  title,
  detail,
  points,
  color,
  maxValue,
  includeSeconds = false,
}: {
  title: string;
  detail: string;
  points: MetricTrendPoint[];
  color: string;
  maxValue: number;
  includeSeconds?: boolean;
}) {
  const [activePoint, setActivePoint] = useState<ActiveTrendPoint | null>(null);
  const width = 360;
  const height = 96;
  const chartLeft = 34;
  const chartRight = 10;
  const chartTop = 10;
  const chartHeight = 48;
  const chartBottom = chartTop + chartHeight;
  const chartWidth = width - chartLeft - chartRight;
  const yMax = Math.max(1, maxValue);
  const ticks = tickIndexesFor(points);

  const xFor = (index: number) =>
    chartLeft + (points.length <= 1 ? chartWidth : (index / (points.length - 1)) * chartWidth);
  const yFor = (value: number) =>
    chartTop + chartHeight - (Math.min(yMax, Math.max(0, value)) / yMax) * chartHeight;
  const path = points.map((point, index) => `${xFor(index)},${yFor(point.value)}`).join(" ");

  return (
    <div className="relative min-w-0">
      <div className="mb-0.5 flex items-center justify-between gap-2">
        <div className="text-xs font-semibold text-slate-950">{title}</div>
        <div className="text-[10px] text-slate-700">{detail}</div>
      </div>
      {activePoint ? (
        <div
          className="pointer-events-none absolute z-10 min-w-40 rounded border border-slate-300 bg-white px-2 py-1 text-xs text-slate-900 shadow"
          style={{
            left: `${Math.min(88, Math.max(12, activePoint.leftPercent))}%`,
            top: `${Math.min(85, Math.max(18, activePoint.topPercent))}%`,
            transform: "translate(-50%, -112%)",
          }}
        >
          <div className="font-mono font-semibold">{activePoint.formattedValue}</div>
          <div className="text-slate-700">{formatTooltipTime(activePoint.timestamp)}</div>
          {activePoint.samples ? (
            <div className="text-slate-700">{activePoint.samples} samples averaged</div>
          ) : null}
        </div>
      ) : null}
      <svg className="h-24 w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${title} resource trend`}>
        <line x1={chartLeft} x2={width - chartRight} y1={chartBottom} y2={chartBottom} stroke="#64748b" />
        <line x1={chartLeft} x2={chartLeft} y1={chartTop} y2={chartBottom} stroke="#64748b" />
        <text x="6" y={chartTop + 4} className="fill-slate-700 text-[10px]">
          max
        </text>
        <text x="6" y={chartTop + 17} className="fill-slate-700 text-[10px]">
          {points[0]?.formattedValue.includes("%") ? `${yMax.toFixed(0)}%` : formatBytes(yMax)}
        </text>
        {points.length > 1 ? (
          <polyline
            points={path}
            fill="none"
            stroke={color}
            strokeWidth="2"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        ) : null}
        {points.map((point, index) => {
          const x = xFor(index);
          const y = yFor(point.value);
          const isActive = activePoint?.timestamp === point.timestamp;

          return (
            <circle
              key={`${point.timestamp}-${index}`}
              cx={x}
              cy={y}
              r={isActive ? 3.3 : 2}
              fill="#ffffff"
              stroke={color}
              strokeWidth={isActive ? 2.4 : 1.8}
              tabIndex={0}
              aria-label={`${title} ${point.formattedValue} at ${formatTooltipTime(point.timestamp)}`}
              onFocus={() =>
                setActivePoint({
                  ...point,
                  leftPercent: (x / width) * 100,
                  topPercent: (y / height) * 100,
                })
              }
              onBlur={() => setActivePoint(null)}
              onMouseEnter={() =>
                setActivePoint({
                  ...point,
                  leftPercent: (x / width) * 100,
                  topPercent: (y / height) * 100,
                })
              }
              onMouseLeave={() => setActivePoint(null)}
            >
              <title>
                {formatTooltipTime(point.timestamp)}: {point.formattedValue}
                {point.samples ? ` (${point.samples} samples averaged)` : ""}
              </title>
            </circle>
          );
        })}
        {ticks.map((index) => {
          const x = xFor(index);
          const anchor = index === 0 ? "start" : index === points.length - 1 ? "end" : "middle";

          return (
            <g key={index}>
              <line x1={x} x2={x} y1={chartBottom} y2={chartBottom + 4} stroke="#64748b" />
              <text x={x} y={chartBottom + 16} textAnchor={anchor} className="fill-slate-700 text-[10px]">
                {formatChartTime(points[index].timestamp, includeSeconds)}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
