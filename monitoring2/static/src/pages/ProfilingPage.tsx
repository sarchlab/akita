import { useCallback, useEffect, useState } from "react";
import { Activity } from "lucide-react";
import { Button } from "../components/ui/button";

interface ResourceResponse {
  cpu_percent: number;
  memory_size: number;
}

interface ResourcePoint extends ResourceResponse {
  timestamp: number;
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
  const [resources, setResources] = useState<ResourceResponse>({ cpu_percent: 0, memory_size: 0 });
  const [history, setHistory] = useState<ResourcePoint[]>([]);

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

        setResources(nextResources);
        setHistory((previous) =>
          [...previous, { ...nextResources, timestamp: Date.now() }].slice(-60),
        );
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1500);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { resources, history };
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
  const { resources, history } = useResourceUsage();
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
            <div className="mb-3 text-sm font-semibold">Resource Usage</div>
            <dl className="grid grid-cols-[8rem_1fr] gap-y-3 text-sm">
              <dt className="text-muted-foreground">CPU</dt>
              <dd className="font-mono">{resources.cpu_percent.toFixed(1)}%</dd>
              <dt className="text-muted-foreground">RSS</dt>
              <dd className="font-mono">{formatBytes(resources.memory_size)}</dd>
            </dl>
            <ResourceTrendChart history={history} />
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
  if (lastPath.length <= 26) {
    return lastPath;
  }

  return `${lastPath.slice(0, 23)}...`;
}

function CallGraph({ graph }: { graph: ProfileCallGraph }) {
  if (!graph.nodes.length) {
    return <div className="text-sm text-muted-foreground">No call graph samples in the captured profile.</div>;
  }

  const width = 1100;
  const left = 40;
  const top = 36;
  const nodeWidth = 190;
  const nodeHeight = 46;
  const rowGap = 74;
  const columnGap = 230;
  const grouped = new Map<number, ProfileCallGraphNode[]>();

  graph.nodes.forEach((node) => {
    const depth = Math.max(0, Math.min(5, node.depth));
    grouped.set(depth, [...(grouped.get(depth) ?? []), node]);
  });

  const depths = [...grouped.keys()].sort((a, b) => a - b);
  const maxRows = Math.max(1, ...depths.map((depth) => grouped.get(depth)?.length ?? 0));
  const height = Math.max(360, top * 2 + maxRows * rowGap);
  const maxNodeValue = Math.max(1, ...graph.nodes.map((node) => node.value));
  const maxEdgeValue = Math.max(1, ...graph.edges.map((edge) => edge.value));
  const positions = new Map<string, { x: number; y: number }>();

  depths.forEach((depth, columnIndex) => {
    const nodes = [...(grouped.get(depth) ?? [])].sort((a, b) => b.value - a.value);
    const columnHeight = (nodes.length - 1) * rowGap;
    const startY = top + ((maxRows - 1) * rowGap - columnHeight) / 2;

    nodes.forEach((node, rowIndex) => {
      positions.set(node.id, {
        x: left + columnIndex * columnGap,
        y: startY + rowIndex * rowGap,
      });
    });
  });

  const visibleEdges = graph.edges.filter((edge) => positions.has(edge.from) && positions.has(edge.to));

  return (
    <div className="overflow-auto rounded border bg-slate-50 p-3">
      <svg
        className="min-h-96 min-w-[900px] overflow-visible"
        viewBox={`0 0 ${Math.max(width, left * 2 + depths.length * columnGap + nodeWidth)} ${height}`}
        role="img"
        aria-label="CPU profile call graph"
      >
        <defs>
          <marker id="call-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" fill="#64748b" />
          </marker>
        </defs>
        {visibleEdges.map((edge) => {
          const from = positions.get(edge.from)!;
          const to = positions.get(edge.to)!;
          const startX = from.x + nodeWidth;
          const startY = from.y + nodeHeight / 2;
          const endX = to.x;
          const endY = to.y + nodeHeight / 2;
          const bend = Math.max(60, (endX - startX) / 2);
          const strokeWidth = 1 + (edge.value / maxEdgeValue) * 4;

          return (
            <path
              key={edge.id}
              d={`M ${startX} ${startY} C ${startX + bend} ${startY}, ${endX - bend} ${endY}, ${endX - 6} ${endY}`}
              fill="none"
              stroke="#64748b"
              strokeOpacity={0.25 + (edge.value / maxEdgeValue) * 0.55}
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

          return (
            <g key={node.id} transform={`translate(${position.x} ${position.y})`}>
              <rect
                width={nodeWidth}
                height={nodeHeight}
                rx="6"
                fill="#ffffff"
                stroke={intensity > 0.66 ? "#0284c7" : "#cbd5e1"}
                strokeWidth={1 + intensity * 2}
              />
              <rect width={Math.max(4, nodeWidth * intensity)} height="4" rx="2" fill="#0284c7" />
              <text x="10" y="21" className="fill-slate-950 text-[12px] font-semibold">
                {shortFunctionName(node.label)}
              </text>
              <text x="10" y="37" className="fill-slate-500 text-[11px]">
                samples {node.value}
              </text>
              <title>
                {node.label}: {node.value}
              </title>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

function formatChartTime(timestamp: number) {
  return new Date(timestamp).toLocaleTimeString([], {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function ResourceTrendChart({ history }: { history: ResourcePoint[] }) {
  const points = history.length ? history : [{ cpu_percent: 0, memory_size: 0, timestamp: Date.now() }];
  const width = 420;
  const height = 154;
  const chartLeft = 42;
  const chartRight = 14;
  const chartTop = 18;
  const chartHeight = 88;
  const chartBottom = chartTop + chartHeight;
  const chartWidth = width - chartLeft - chartRight;
  const maxCPU = Math.max(100, ...points.map((point) => point.cpu_percent));
  const maxMemory = Math.max(1, ...points.map((point) => point.memory_size));
  const tickIndexes = Array.from(
    new Set([0, Math.floor((points.length - 1) / 2), points.length - 1]),
  ).filter((index) => index >= 0);

  const xFor = (index: number) =>
    chartLeft + (points.length <= 1 ? chartWidth : (index / (points.length - 1)) * chartWidth);
  const yFor = (value: number, max: number) =>
    chartTop + chartHeight - (Math.min(max, Math.max(0, value)) / max) * chartHeight;
  const cpuPath = points
    .map((point, index) => `${xFor(index)},${yFor(point.cpu_percent, maxCPU)}`)
    .join(" ");
  const memoryPath = points
    .map((point, index) => `${xFor(index)},${yFor(point.memory_size, maxMemory)}`)
    .join(" ");

  return (
    <div className="mt-4 rounded border bg-slate-50 p-3">
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="text-xs font-semibold">Resource Trend</div>
        <div className="text-xs text-muted-foreground">Last {points.length} samples</div>
      </div>
      <svg className="h-40 w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="CPU and memory trend chart">
        <line x1={chartLeft} x2={width - chartRight} y1={chartBottom} y2={chartBottom} stroke="#cbd5e1" />
        <line x1={chartLeft} x2={chartLeft} y1={chartTop} y2={chartBottom} stroke="#cbd5e1" />
        <text x="6" y={chartTop + 4} className="fill-slate-500 text-[10px]">
          max
        </text>
        <text x="6" y={chartTop + 18} className="fill-sky-700 text-[10px]">
          {maxCPU.toFixed(0)}%
        </text>
        <text x="6" y={chartTop + 32} className="fill-amber-700 text-[10px]">
          {formatBytes(maxMemory)}
        </text>
        <polyline points={cpuPath} fill="none" stroke="#0284c7" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        <polyline points={memoryPath} fill="none" stroke="#f59e0b" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        <circle
          cx={xFor(points.length - 1)}
          cy={yFor(points[points.length - 1].cpu_percent, maxCPU)}
          r="4"
          fill="#0284c7"
        />
        <circle
          cx={xFor(points.length - 1)}
          cy={yFor(points[points.length - 1].memory_size, maxMemory)}
          r="4"
          fill="#f59e0b"
        />
        {tickIndexes.map((index) => {
          const x = xFor(index);
          const anchor = index === 0 ? "start" : index === points.length - 1 ? "end" : "middle";

          return (
            <g key={index}>
              <line x1={x} x2={x} y1={chartBottom} y2={chartBottom + 4} stroke="#94a3b8" />
              <text x={x} y={chartBottom + 18} textAnchor={anchor} className="fill-slate-500 text-[10px]">
                {formatChartTime(points[index].timestamp)}
              </text>
            </g>
          );
        })}
        <text x={chartLeft + chartWidth / 2} y={height - 4} textAnchor="middle" className="fill-slate-500 text-[10px]">
          Time
        </text>
      </svg>
      <div className="flex gap-4 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-2">
          <span className="h-2 w-4 rounded-full bg-sky-600" /> CPU
        </span>
        <span className="inline-flex items-center gap-2">
          <span className="h-2 w-4 rounded-full bg-amber-500" /> RSS
        </span>
      </div>
    </div>
  );
}
