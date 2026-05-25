import { useCallback, useEffect, useState } from "react";
import { Activity, RefreshCcw } from "lucide-react";
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
}

interface ProfileFunctionStat {
  name: string;
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

  return { resources, history, refresh };
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

function profileFunctionName(sample: unknown) {
  if (!sample || typeof sample !== "object") {
    return "unknown";
  }

  const sampleObject = sample as Record<string, unknown>;
  const locations = getArray(sampleObject, "location", "Location");
  const leaf = locations[0];
  if (!leaf || typeof leaf !== "object") {
    return "unknown";
  }

  const leafObject = leaf as Record<string, unknown>;
  const lines = getArray(leafObject, "line", "Line");
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

  const address = leafObject.address ?? leafObject.Address;
  if (typeof address === "number") {
    return `0x${address.toString(16)}`;
  }

  return "unknown";
}

function summarizeProfile(profile: unknown): ProfileSummary {
  if (!profile || typeof profile !== "object") {
    return { samples: 0, locations: 0, functions: 0, topFunctions: [] };
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
    const values = getNumberArray(sampleObject, "value", "Value");
    const weight = Math.max(1, ...values.map((value) => Math.abs(value)));
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
  };
}

export default function ProfilingPage() {
  const { resources, history, refresh } = useResourceUsage();
  const [profileStatus, setProfileStatus] = useState("");
  const [profileSummary, setProfileSummary] = useState<ProfileSummary | null>(null);

  const captureProfile = async () => {
    setProfileStatus("Capturing CPU profile...");
    try {
      const response = await fetch("/api/profile");
      if (!response.ok) {
        throw new Error(`${response.status} ${response.statusText}`);
      }

      const profile = await response.json();
      setProfileSummary(summarizeProfile(profile));
      setProfileStatus("Profile captured");
    } catch (err) {
      setProfileStatus(err instanceof Error ? err.message : "Profile capture failed");
    }
  };

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <Activity className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Profiling</h1>
            <div className="text-xs text-muted-foreground">CPU, memory, and on-demand CPU profile capture</div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            <RefreshCcw /> Refresh
          </Button>
          <Button type="button" size="sm" onClick={captureProfile}>
            <Activity /> Capture CPU Profile
          </Button>
        </header>

        <section className="grid gap-4 md:grid-cols-2">
          <div className="rounded border bg-white p-4">
            <div className="mb-3 text-sm font-semibold">Resource Usage</div>
            <dl className="grid grid-cols-[8rem_1fr] gap-y-3 text-sm">
              <dt className="text-muted-foreground">CPU</dt>
              <dd className="font-mono">{resources.cpu_percent.toFixed(1)}%</dd>
              <dt className="text-muted-foreground">RSS</dt>
              <dd className="font-mono">{formatBytes(resources.memory_size)}</dd>
            </dl>
          </div>

          <div className="rounded border bg-white p-4">
            <div className="mb-3 text-sm font-semibold">Latest CPU Profile</div>
            {profileStatus ? <div className="mb-3 text-sm text-muted-foreground">{profileStatus}</div> : null}
            {profileSummary ? (
              <ProfileMetricBars summary={profileSummary} />
            ) : (
              <div className="text-sm text-muted-foreground">No profile captured yet.</div>
            )}
          </div>
        </section>

        <ResourceTrendChart history={history} />

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

function ResourceTrendChart({ history }: { history: ResourcePoint[] }) {
  const points = history.length ? history : [{ cpu_percent: 0, memory_size: 0, timestamp: Date.now() }];
  const width = 720;
  const height = 260;
  const chartLeft = 56;
  const chartRight = 24;
  const cpuTop = 28;
  const cpuHeight = 76;
  const memoryTop = 150;
  const memoryHeight = 76;
  const chartWidth = width - chartLeft - chartRight;
  const maxCPU = Math.max(100, ...points.map((point) => point.cpu_percent));
  const maxMemory = Math.max(1, ...points.map((point) => point.memory_size));

  const xFor = (index: number) =>
    chartLeft + (points.length <= 1 ? chartWidth : (index / (points.length - 1)) * chartWidth);
  const yFor = (value: number, max: number, top: number, graphHeight: number) =>
    top + graphHeight - (Math.min(max, Math.max(0, value)) / max) * graphHeight;
  const cpuPath = points
    .map((point, index) => `${xFor(index)},${yFor(point.cpu_percent, maxCPU, cpuTop, cpuHeight)}`)
    .join(" ");
  const memoryPath = points
    .map((point, index) => `${xFor(index)},${yFor(point.memory_size, maxMemory, memoryTop, memoryHeight)}`)
    .join(" ");

  return (
    <div className="rounded border bg-white p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="text-sm font-semibold">Resource Trend</div>
        <div className="text-xs text-muted-foreground">Last {points.length} samples</div>
      </div>
      <svg className="h-72 w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="CPU and memory trend chart">
        <line x1={chartLeft} x2={width - chartRight} y1={cpuTop + cpuHeight} y2={cpuTop + cpuHeight} stroke="#cbd5e1" />
        <line x1={chartLeft} x2={width - chartRight} y1={memoryTop + memoryHeight} y2={memoryTop + memoryHeight} stroke="#cbd5e1" />
        <text x="8" y={cpuTop + 12} className="fill-slate-500 text-[12px]">
          CPU
        </text>
        <text x="8" y={cpuTop + cpuHeight} className="fill-slate-500 text-[11px]">
          {maxCPU.toFixed(0)}%
        </text>
        <text x="8" y={memoryTop + 12} className="fill-slate-500 text-[12px]">
          RSS
        </text>
        <text x="8" y={memoryTop + memoryHeight} className="fill-slate-500 text-[11px]">
          {formatBytes(maxMemory)}
        </text>
        <polyline points={cpuPath} fill="none" stroke="#0284c7" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        <polyline points={memoryPath} fill="none" stroke="#f59e0b" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        <circle
          cx={xFor(points.length - 1)}
          cy={yFor(points[points.length - 1].cpu_percent, maxCPU, cpuTop, cpuHeight)}
          r="4"
          fill="#0284c7"
        />
        <circle
          cx={xFor(points.length - 1)}
          cy={yFor(points[points.length - 1].memory_size, maxMemory, memoryTop, memoryHeight)}
          r="4"
          fill="#f59e0b"
        />
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
