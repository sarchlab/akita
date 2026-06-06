import { useEffect, useMemo, useRef, useState, type PointerEvent } from "react";
import { Activity, RotateCcw, ZoomIn, ZoomOut } from "lucide-react";
import { Button } from "../components/ui/button";
import {
  MAX_MINUTE_SAMPLES,
  MAX_SECOND_SAMPLES,
  type MinuteResourcePoint,
  type ResourcePoint,
  useResourceUsageHistory,
} from "../hooks/useResourceUsageHistory";

interface ProfileSummary {
  samples: number;
  locations: number;
  functions: number;
  valueInfo: ProfileValueInfo;
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
  indirect: boolean;
  skippedFrames: number;
}

interface ProfileValueInfo {
  index: number;
  type: string;
  unit: string;
  label: string;
}

const CALL_GRAPH_MIN_SCALE = 0.35;
const CALL_GRAPH_MAX_SCALE = 4;
const CALL_GRAPH_BUTTON_ZOOM_STEP = 1.2;
const CALL_GRAPH_WHEEL_ZOOM_RATE = 0.0012;
const CALL_GRAPH_MAX_WHEEL_DELTA = 80;
const DEFAULT_PROFILE_VALUE_INFO: ProfileValueInfo = {
  index: 0,
  type: "samples",
  unit: "count",
  label: "samples",
};

interface CallGraphViewport {
  scale: number;
  x: number;
  y: number;
}

const INITIAL_CALL_GRAPH_VIEWPORT: CallGraphViewport = { scale: 1, x: 0, y: 0 };
type ProfileResultTab = "graph" | "top-functions";
type ProfileKind = "cpu" | "heap";
// "single" views one snapshot (absolute); "compare" diffs a base against a target.
type HeapViewMode = "single" | "compare";

// A captured heap profile the user can keep around and pick as a diff baseline.
interface HeapSnapshot {
  id: string;
  label: string;
  profile: unknown;
}

// Cap retained snapshots so a long session does not pile up large profiles in
// memory; the oldest is dropped past this limit.
const MAX_HEAP_SNAPSHOTS = 10;

interface HeapSampleType {
  value: string;
  label: string;
}

// The standard sample types carried by a Go heap profile. The frontend lets the
// user switch between them without re-capturing; inuse_space (live bytes) is the
// most common starting point.
const HEAP_SAMPLE_TYPES: HeapSampleType[] = [
  { value: "inuse_space", label: "In-use space" },
  { value: "inuse_objects", label: "In-use objects" },
  { value: "alloc_space", label: "Allocated space" },
  { value: "alloc_objects", label: "Allocated objects" },
];
const DEFAULT_HEAP_SAMPLE_TYPE = HEAP_SAMPLE_TYPES[0].value;

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

function getStringField(value: Record<string, unknown>, lower: string, upper: string) {
  const field = value[lower] ?? value[upper];
  return typeof field === "string" ? field : "";
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

function isTimeUnit(unit: string) {
  return ["nanoseconds", "microseconds", "milliseconds", "seconds"].includes(unit.toLowerCase());
}

function isByteUnit(unit: string) {
  return ["byte", "bytes"].includes(unit.toLowerCase());
}

function profileValueLabel(type: string, unit: string) {
  const normalizedType = type.toLowerCase();
  const normalizedUnit = unit.toLowerCase();

  if (isTimeUnit(normalizedUnit)) {
    return normalizedType === "cpu" ? "CPU" : type || "time";
  }

  if (isByteUnit(normalizedUnit)) {
    return type || "bytes";
  }

  if (normalizedType === "samples") {
    return "samples";
  }

  if (normalizedUnit === "count") {
    return type || "samples";
  }

  return type || "value";
}

function profileValueInfo(profile: Record<string, unknown>, preferredSampleType?: string): ProfileValueInfo {
  const sampleTypes = [
    ...getArray(profile, "sampleType", "SampleType"),
    ...getArray(profile, "sample_type", "Sample_type"),
  ];

  if (!sampleTypes.length) {
    return DEFAULT_PROFILE_VALUE_INFO;
  }

  const types = sampleTypes.map((item) => {
    const valueType = item && typeof item === "object" ? (item as Record<string, unknown>) : {};
    return {
      type: getStringField(valueType, "type", "Type"),
      unit: getStringField(valueType, "unit", "Unit"),
    };
  });
  // An explicit selection (e.g. a heap inuse/alloc choice) wins when present.
  const preferredIndex = preferredSampleType
    ? types.findIndex((item) => item.type.toLowerCase() === preferredSampleType.toLowerCase())
    : -1;
  const cpuIndex = types.findIndex((item) => item.type.toLowerCase() === "cpu");
  const timeIndex = types.findIndex((item) => isTimeUnit(item.unit));
  const sampleIndex = types.findIndex((item) => {
    return item.type.toLowerCase() === "samples" || item.unit.toLowerCase() === "count";
  });
  const index =
    preferredIndex >= 0
      ? preferredIndex
      : cpuIndex >= 0
        ? cpuIndex
        : timeIndex >= 0
          ? timeIndex
          : sampleIndex >= 0
            ? sampleIndex
            : 0;
  const selected = types[index] ?? DEFAULT_PROFILE_VALUE_INFO;

  return {
    index,
    type: selected.type || DEFAULT_PROFILE_VALUE_INFO.type,
    unit: selected.unit || DEFAULT_PROFILE_VALUE_INFO.unit,
    label: profileValueLabel(selected.type, selected.unit),
  };
}

function sampleWeight(sample: Record<string, unknown>, valueInfo: ProfileValueInfo) {
  const values = getNumberArray(sample, "value", "Value");
  const selected = values[valueInfo.index];

  if (typeof selected === "number" && Number.isFinite(selected)) {
    return Math.max(0, Math.abs(selected));
  }

  return 1;
}

function formatDurationSeconds(seconds: number) {
  if (seconds < 0.001) {
    return `${(seconds * 1000000).toFixed(0)}us`;
  }

  if (seconds < 1) {
    return `${(seconds * 1000).toFixed(seconds < 0.01 ? 1 : 0)}ms`;
  }

  if (seconds < 60) {
    return `${seconds.toFixed(seconds < 10 ? 1 : 0)}s`;
  }

  return `${(seconds / 60).toFixed(seconds < 600 ? 1 : 0)}min`;
}

function formatProfileValue(value: number, valueInfo: ProfileValueInfo) {
  const unit = valueInfo.unit.toLowerCase();

  if (unit === "nanoseconds") {
    return formatDurationSeconds(value / 1000000000);
  }

  if (unit === "microseconds") {
    return formatDurationSeconds(value / 1000000);
  }

  if (unit === "milliseconds") {
    return formatDurationSeconds(value / 1000);
  }

  if (unit === "seconds") {
    return formatDurationSeconds(value);
  }

  if (isByteUnit(unit)) {
    return formatBytes(value);
  }

  return formatSampleCount(value);
}

function buildCallGraph(samples: unknown[], valueInfo: ProfileValueInfo): ProfileCallGraph {
  const nodeTotals = new Map<string, { value: number; depthTotal: number; count: number }>();
  const stacks: { stack: string[]; weight: number }[] = [];

  samples.forEach((item) => {
    if (!item || typeof item !== "object") {
      return;
    }

    const sample = item as Record<string, unknown>;
    const weight = sampleWeight(sample, valueInfo);
    const leafFirstStack = profileStackNames(sample);
    const callerFirstStack = [...leafFirstStack].reverse().filter((name, index, stack) => {
      return index === 0 || name !== stack[index - 1];
    });

    if (!callerFirstStack.length) {
      return;
    }

    stacks.push({ stack: callerFirstStack, weight });

    callerFirstStack.forEach((name, depth) => {
      const node = nodeTotals.get(name) ?? { value: 0, depthTotal: 0, count: 0 };
      node.value += weight;
      node.depthTotal += depth;
      node.count += 1;
      nodeTotals.set(name, node);
    });
  });

  const selectedNodeIDs = new Set(
    [...nodeTotals.entries()]
      .sort((a, b) => b[1].value - a[1].value)
      .slice(0, 24)
      .map(([id]) => id),
  );
  const edgeTotals = new Map<string, ProfileCallGraphEdge>();

  stacks.forEach(({ stack, weight }) => {
    let previousSelected: { name: string; index: number } | null = null;

    stack.forEach((name, index) => {
      if (!selectedNodeIDs.has(name)) {
        return;
      }

      if (previousSelected && previousSelected.name !== name) {
        const skippedFrames = index - previousSelected.index - 1;
        const id = `${previousSelected.name}->${name}`;
        const edge =
          edgeTotals.get(id) ??
          {
            id,
            from: previousSelected.name,
            to: name,
            value: 0,
            indirect: false,
            skippedFrames: 0,
          };

        edge.value += weight;
        edge.indirect = edge.indirect || skippedFrames > 0;
        edge.skippedFrames = Math.max(edge.skippedFrames, skippedFrames);
        edgeTotals.set(id, edge);
      }

      previousSelected = { name, index };
    });
  });

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

function summarizeProfile(profile: unknown, preferredSampleType?: string): ProfileSummary {
  if (!profile || typeof profile !== "object") {
    return {
      samples: 0,
      locations: 0,
      functions: 0,
      valueInfo: DEFAULT_PROFILE_VALUE_INFO,
      topFunctions: [],
      callGraph: { nodes: [], edges: [] },
    };
  }

  const p = profile as Record<string, unknown>;
  const sample = Array.isArray(p.sample) ? p.sample : Array.isArray(p.Sample) ? p.Sample : [];
  const location = Array.isArray(p.location) ? p.location : Array.isArray(p.Location) ? p.Location : [];
  const fn = Array.isArray(p.function) ? p.function : Array.isArray(p.Function) ? p.Function : [];
  const valueInfo = profileValueInfo(p, preferredSampleType);
  const functionTotals = new Map<string, number>();

  sample.forEach((item) => {
    if (!item || typeof item !== "object") {
      return;
    }

    const sampleObject = item as Record<string, unknown>;
    const weight = sampleWeight(sampleObject, valueInfo);
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
    valueInfo,
    topFunctions,
    callGraph: buildCallGraph(sample, valueInfo),
  };
}

function profileSamples(profile: unknown): Record<string, unknown>[] {
  if (!profile || typeof profile !== "object") {
    return [];
  }

  const p = profile as Record<string, unknown>;
  const sample = Array.isArray(p.sample) ? p.sample : Array.isArray(p.Sample) ? p.Sample : [];
  return sample.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object");
}

// indexByStack folds a profile's samples into per-call-stack value totals,
// keyed by the leaf-first function stack the call graph already uses for
// identity. A representative location chain is kept so the diff result can be
// rendered by the existing views.
function indexByStack(samples: Record<string, unknown>[]) {
  const totals = new Map<string, { values: number[]; location: unknown[] }>();

  for (const sample of samples) {
    const names = profileStackNames(sample);
    if (!names.length) {
      continue;
    }

    const key = names.join(";");
    const values = getNumberArray(sample, "value", "Value");
    const location = getArray(sample, "location", "Location");
    const entry = totals.get(key) ?? { values: [], location };

    for (let index = 0; index < values.length; index++) {
      entry.values[index] = (entry.values[index] ?? 0) + values[index];
    }
    if (!entry.location.length) {
      entry.location = location;
    }
    totals.set(key, entry);
  }

  return totals;
}

// diffHeapProfiles returns a synthetic profile of (current - baseline), summing
// each call stack's sample values elementwise so every heap sample type stays in
// sync. The result is shaped like a parsed profile so summarizeProfile can
// consume it directly. This mirrors `go tool pprof -base` at function-stack
// granularity (the precision the call graph renders at).
function diffHeapProfiles(current: unknown, baseline: unknown): Record<string, unknown> {
  const currentByStack = indexByStack(profileSamples(current));
  const baselineByStack = indexByStack(profileSamples(baseline));
  const deltaSamples: Record<string, unknown>[] = [];

  for (const key of new Set([...currentByStack.keys(), ...baselineByStack.keys()])) {
    const cur = currentByStack.get(key);
    const base = baselineByStack.get(key);
    const length = Math.max(cur?.values.length ?? 0, base?.values.length ?? 0);
    const values: number[] = [];

    for (let index = 0; index < length; index++) {
      values.push((cur?.values[index] ?? 0) - (base?.values[index] ?? 0));
    }

    if (values.every((value) => value === 0)) {
      continue;
    }

    deltaSamples.push({ location: cur?.location ?? base?.location ?? [], value: values });
  }

  const p = (current && typeof current === "object" ? current : {}) as Record<string, unknown>;
  return {
    sampleType: getArray(p, "sampleType", "SampleType"),
    sample: deltaSamples,
    location: getArray(p, "location", "Location"),
    function: getArray(p, "function", "Function"),
  };
}

// Prefer the server's explanatory body (e.g. "the program may already be
// CPU-profiled ...") over a bare status line for a failed capture.
async function captureErrorMessage(response: Response): Promise<string> {
  const text = await response.text().catch(() => "");
  return text.trim() || `${response.status} ${response.statusText}`;
}

export default function ProfilingPage() {
  const { history } = useResourceUsageHistory();
  const [profileSeconds, setProfileSeconds] = useState(1);
  const [profileStatus, setProfileStatus] = useState("");
  const [cpuProfile, setCpuProfile] = useState<unknown>(null);
  const [profileKind, setProfileKind] = useState<ProfileKind | null>(null);
  const [heapSnapshots, setHeapSnapshots] = useState<HeapSnapshot[]>([]);
  const [heapSampleType, setHeapSampleType] = useState(DEFAULT_HEAP_SAMPLE_TYPE);
  const [heapViewMode, setHeapViewMode] = useState<HeapViewMode>("single");
  const [singleSnapshotId, setSingleSnapshotId] = useState<string | null>(null);
  const [baseSnapshotId, setBaseSnapshotId] = useState<string | null>(null);
  const [targetSnapshotId, setTargetSnapshotId] = useState<string | null>(null);
  const [isCapturing, setIsCapturing] = useState(false);
  const [captureFailed, setCaptureFailed] = useState(false);
  const [activeProfileTab, setActiveProfileTab] = useState<ProfileResultTab>("graph");
  const heapIdRef = useRef(0);

  // Capture only collects snapshots; which snapshot(s) to view and whether to
  // diff them is chosen in the results panel below. Selections resolve to the
  // latest/previous snapshot when unset or evicted, so they stay valid.
  const findSnapshot = (id: string | null) => heapSnapshots.find((snapshot) => snapshot.id === id) ?? null;
  const latestSnapshot = heapSnapshots[heapSnapshots.length - 1] ?? null;
  const previousSnapshot = heapSnapshots.length >= 2 ? heapSnapshots[heapSnapshots.length - 2] : null;
  const singleSnapshot = findSnapshot(singleSnapshotId) ?? latestSnapshot;
  const targetSnapshot = findSnapshot(targetSnapshotId) ?? latestSnapshot;
  const baseSnapshot = findSnapshot(baseSnapshotId) ?? previousSnapshot;
  const heapIsDelta =
    profileKind === "heap" &&
    heapViewMode === "compare" &&
    baseSnapshot != null &&
    targetSnapshot != null &&
    baseSnapshot.id !== targetSnapshot.id;

  // Resolve the profile to summarize from the current selection. Heap diffs are
  // computed in the browser, so switching snapshots, mode, or sample type is
  // instant and never re-captures.
  const profileSummary = useMemo(() => {
    if (profileKind === "cpu") {
      return cpuProfile ? summarizeProfile(cpuProfile) : null;
    }
    if (profileKind === "heap") {
      if (heapViewMode === "compare") {
        if (heapIsDelta && baseSnapshot && targetSnapshot) {
          return summarizeProfile(diffHeapProfiles(targetSnapshot.profile, baseSnapshot.profile), heapSampleType);
        }
        return targetSnapshot ? summarizeProfile(targetSnapshot.profile, heapSampleType) : null;
      }
      return singleSnapshot ? summarizeProfile(singleSnapshot.profile, heapSampleType) : null;
    }
    return null;
  }, [profileKind, cpuProfile, heapViewMode, heapIsDelta, baseSnapshot, targetSnapshot, singleSnapshot, heapSampleType]);

  const captureProfile = async () => {
    setIsCapturing(true);
    setCaptureFailed(false);
    setProfileStatus(`Capturing ${profileSeconds}s CPU profile...`);
    try {
      const response = await fetch(`/api/profile?seconds=${profileSeconds}`);
      if (!response.ok) {
        throw new Error(await captureErrorMessage(response));
      }

      const profile = await response.json();
      setCpuProfile(profile);
      setProfileKind("cpu");
      setProfileStatus("CPU profile captured");
    } catch (err) {
      setCaptureFailed(true);
      setProfileStatus(`CPU capture failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setIsCapturing(false);
    }
  };

  const captureHeapProfile = async () => {
    setIsCapturing(true);
    setCaptureFailed(false);
    setProfileStatus("Capturing heap profile...");
    try {
      const response = await fetch(`/api/heap?gc=1`);
      if (!response.ok) {
        throw new Error(await captureErrorMessage(response));
      }

      const profile = await response.json();
      const id = `heap-${(heapIdRef.current += 1)}`;
      const label = `Snapshot ${heapIdRef.current} · ${new Date().toLocaleTimeString([], { hour12: false })}`;
      setHeapSnapshots((previous) => {
        const next = [...previous, { id, label, profile }];
        return next.length > MAX_HEAP_SNAPSHOTS ? next.slice(next.length - MAX_HEAP_SNAPSHOTS) : next;
      });
      // Point Single (and Compare's target) at the snapshot just captured.
      setSingleSnapshotId(id);
      setTargetSnapshotId(id);
      setProfileKind("heap");
      setProfileStatus("Heap profile captured");
    } catch (err) {
      setCaptureFailed(true);
      setProfileStatus(`Heap capture failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setIsCapturing(false);
    }
  };

  const profileKindLabel = profileKind === "heap" ? (heapIsDelta ? "Heap Comparison" : "Heap") : "CPU";

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <section className="rounded border bg-white p-3">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="w-9 text-xs font-semibold text-slate-700">CPU</span>
              <label className="text-[10px] font-medium text-slate-600" htmlFor="profile-seconds">
                Seconds
              </label>
              <select
                id="profile-seconds"
                className="h-7 rounded border border-input bg-background px-2 text-xs"
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
              <Button
                type="button"
                size="sm"
                className="h-7 px-2 text-xs"
                onClick={captureProfile}
                disabled={isCapturing}
              >
                <Activity /> Capture CPU Profile
              </Button>
            </div>

            <div className="hidden h-7 w-px self-center bg-slate-200 sm:block" aria-hidden="true" />

            <div className="flex flex-wrap items-center gap-2">
              <span className="w-9 text-xs font-semibold text-slate-700">Heap</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="h-7 px-2 text-xs"
                onClick={captureHeapProfile}
                disabled={isCapturing}
              >
                <Activity /> Capture Heap Profile
              </Button>
            </div>
          </div>
          {profileStatus ? (
            <div
              className={`mt-2 text-xs ${captureFailed ? "font-medium text-red-600" : "text-muted-foreground"}`}
              role={captureFailed ? "alert" : undefined}
            >
              {profileStatus}
            </div>
          ) : null}
        </section>

        <section className="rounded border bg-white p-3">
          <ResourceTrendChart secondHistory={history.seconds} minuteHistory={history.minutes} />
        </section>

        <section className="rounded border bg-white p-4">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="text-sm font-semibold">{profileKindLabel} Call Graph</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {profileSummary
                  ? profileSummaryText(profileSummary)
                  : "Capture a CPU or heap profile to populate the profile views."}
              </div>
            </div>
            <div className="inline-flex rounded border bg-slate-100 p-0.5" role="tablist" aria-label="Profile views">
              <button
                type="button"
                role="tab"
                aria-selected={activeProfileTab === "graph"}
                className={`rounded px-3 py-1 text-xs font-medium ${
                  activeProfileTab === "graph" ? "bg-white text-slate-950 shadow-sm" : "text-slate-700"
                }`}
                onClick={() => setActiveProfileTab("graph")}
              >
                Graph
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeProfileTab === "top-functions"}
                className={`rounded px-3 py-1 text-xs font-medium ${
                  activeProfileTab === "top-functions" ? "bg-white text-slate-950 shadow-sm" : "text-slate-700"
                }`}
                onClick={() => setActiveProfileTab("top-functions")}
              >
                Top Functions
              </button>
            </div>
          </div>

          {profileKind === "heap" ? (
            <div className="mb-3 flex flex-wrap items-center gap-2 border-t border-slate-100 pt-3">
              <div className="inline-flex rounded border bg-slate-100 p-0.5" role="tablist" aria-label="Heap view mode">
                <button
                  type="button"
                  role="tab"
                  aria-selected={heapViewMode === "single"}
                  className={`rounded px-3 py-1 text-xs font-medium ${
                    heapViewMode === "single" ? "bg-white text-slate-950 shadow-sm" : "text-slate-700"
                  }`}
                  onClick={() => setHeapViewMode("single")}
                >
                  Single
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={heapViewMode === "compare"}
                  className={`rounded px-3 py-1 text-xs font-medium ${
                    heapViewMode === "compare" ? "bg-white text-slate-950 shadow-sm" : "text-slate-700"
                  }`}
                  onClick={() => setHeapViewMode("compare")}
                >
                  Compare
                </button>
              </div>

              {heapViewMode === "single" ? (
                <>
                  <label className="text-[10px] font-medium text-slate-600" htmlFor="heap-single">
                    Snapshot
                  </label>
                  <select
                    id="heap-single"
                    className="h-7 rounded border border-input bg-background px-2 text-xs"
                    value={singleSnapshot?.id ?? ""}
                    onChange={(event) => setSingleSnapshotId(event.target.value || null)}
                  >
                    {heapSnapshots.map((snapshot) => (
                      <option key={snapshot.id} value={snapshot.id}>
                        {snapshot.label}
                        {snapshot.id === latestSnapshot?.id ? " (latest)" : ""}
                      </option>
                    ))}
                  </select>
                </>
              ) : (
                <>
                  <label className="text-[10px] font-medium text-slate-600" htmlFor="heap-base">
                    Base
                  </label>
                  <select
                    id="heap-base"
                    className="h-7 rounded border border-input bg-background px-2 text-xs"
                    value={baseSnapshot?.id ?? ""}
                    onChange={(event) => setBaseSnapshotId(event.target.value || null)}
                  >
                    <option value="">—</option>
                    {heapSnapshots.map((snapshot) => (
                      <option key={snapshot.id} value={snapshot.id}>
                        {snapshot.label}
                      </option>
                    ))}
                  </select>
                  <span className="text-xs text-slate-500" aria-hidden="true">→</span>
                  <label className="text-[10px] font-medium text-slate-600" htmlFor="heap-target">
                    Target
                  </label>
                  <select
                    id="heap-target"
                    className="h-7 rounded border border-input bg-background px-2 text-xs"
                    value={targetSnapshot?.id ?? ""}
                    onChange={(event) => setTargetSnapshotId(event.target.value || null)}
                  >
                    {heapSnapshots.map((snapshot) => (
                      <option key={snapshot.id} value={snapshot.id}>
                        {snapshot.label}
                        {snapshot.id === latestSnapshot?.id ? " (latest)" : ""}
                      </option>
                    ))}
                  </select>
                </>
              )}

              <label className="text-[10px] font-medium text-slate-600" htmlFor="heap-sample-type">
                Sample
              </label>
              <select
                id="heap-sample-type"
                className="h-7 rounded border border-input bg-background px-2 text-xs"
                value={heapSampleType}
                onChange={(event) => setHeapSampleType(event.target.value)}
              >
                {HEAP_SAMPLE_TYPES.map((sampleType) => (
                  <option key={sampleType.value} value={sampleType.value}>
                    {sampleType.label}
                  </option>
                ))}
              </select>
            </div>
          ) : null}

          {profileSummary ? (
            activeProfileTab === "graph" ? (
              <CallGraph graph={profileSummary.callGraph} valueInfo={profileSummary.valueInfo} />
            ) : (
              <TopFunctionBars functions={profileSummary.topFunctions} valueInfo={profileSummary.valueInfo} />
            )
          ) : (
            <div className="text-sm text-muted-foreground">Capture a CPU or heap profile to generate a call graph.</div>
          )}
        </section>
      </div>
    </div>
  );
}

function profileSummaryText(summary: ProfileSummary) {
  return `${summary.samples} samples | ${summary.locations} locations | ${summary.functions} functions`;
}

function TopFunctionBars({
  functions,
  valueInfo,
}: {
  functions: ProfileFunctionStat[];
  valueInfo: ProfileValueInfo;
}) {
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
            <span className="shrink-0 font-mono text-muted-foreground">
              {formatProfileValue(fn.value, valueInfo)}
            </span>
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

function CallGraph({ graph, valueInfo }: { graph: ProfileCallGraph; valueInfo: ProfileValueInfo }) {
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
    return (
      <div className="text-sm text-muted-foreground">
        No samples in the captured profile. For CPU, the program may have been idle during the capture window — try a
        longer duration or capture while the simulation is running.
      </div>
    );
  }

  const left = 32;
  const top = 28;
  const nodeWidth = 220;
  const nodeHeight = 44;
  const rowGap = 58;
  const columnGap = 250;
  const componentGap = 36;
  const maxVisibleDepth = Math.min(10, Math.max(0, ...graph.nodes.map((node) => node.depth)));
  const depthFor = (node: ProfileCallGraphNode) => Math.max(0, Math.min(maxVisibleDepth, node.depth));
  const nodesByID = new Map(graph.nodes.map((node) => [node.id, node]));
  const incomingNodeIDs = new Set(graph.edges.map((edge) => edge.to));
  const outgoing = new Map<string, ProfileCallGraphEdge[]>();
  const visibleEdges = graph.edges.filter((edge) => nodesByID.has(edge.from) && nodesByID.has(edge.to));

  visibleEdges.forEach((edge) => {
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

  const adjacency = new Map<string, Set<string>>();
  graph.nodes.forEach((node) => {
    adjacency.set(node.id, new Set());
  });

  visibleEdges.forEach((edge) => {
    adjacency.get(edge.from)?.add(edge.to);
    adjacency.get(edge.to)?.add(edge.from);
  });

  const unvisitedNodeIDs = new Set(graph.nodes.map((node) => node.id));
  const components: ProfileCallGraphNode[][] = [];

  while (unvisitedNodeIDs.size) {
    const startID = unvisitedNodeIDs.values().next().value as string;
    const componentIDs = new Set<string>();
    const queue = [startID];

    unvisitedNodeIDs.delete(startID);

    while (queue.length) {
      const nodeID = queue.shift()!;
      componentIDs.add(nodeID);

      adjacency.get(nodeID)?.forEach((neighborID) => {
        if (!unvisitedNodeIDs.has(neighborID)) {
          return;
        }

        unvisitedNodeIDs.delete(neighborID);
        queue.push(neighborID);
      });
    }

    components.push(
      [...componentIDs]
        .map((id) => nodesByID.get(id))
        .filter((node): node is ProfileCallGraphNode => Boolean(node)),
    );
  }

  components.sort((a, b) => {
    const aValue = Math.max(0, ...a.map((node) => node.value));
    const bValue = Math.max(0, ...b.map((node) => node.value));
    const aDepth = Math.min(...a.map(depthFor));
    const bDepth = Math.min(...b.map(depthFor));
    return bValue - aValue || aDepth - bDepth;
  });

  const maxNodeValue = Math.max(1, ...graph.nodes.map((node) => node.value));
  const maxEdgeValue = Math.max(1, ...graph.edges.map((edge) => edge.value));
  const positions = new Map<string, { x: number; y: number }>();
  let maxColumnIndex = 0;
  let nextComponentTop = top;

  components.forEach((component) => {
    const minDepth = Math.min(...component.map(depthFor));
    const grouped = new Map<number, ProfileCallGraphNode[]>();

    component.forEach((node) => {
      const localDepth = depthFor(node) - minDepth;
      grouped.set(localDepth, [...(grouped.get(localDepth) ?? []), node]);
    });

    const depths = [...grouped.keys()].sort((a, b) => a - b);
    const maxRows = Math.max(1, ...depths.map((depth) => grouped.get(depth)?.length ?? 0));

    depths.forEach((depth) => {
      const nodes = [...(grouped.get(depth) ?? [])].sort((a, b) => {
        const hotPathSort = Number(hotPathNodeIDs.has(b.id)) - Number(hotPathNodeIDs.has(a.id));
        return hotPathSort || b.value - a.value;
      });

      nodes.forEach((node, rowIndex) => {
        positions.set(node.id, {
          x: left + depth * columnGap,
          y: nextComponentTop + rowIndex * rowGap,
        });
      });

      maxColumnIndex = Math.max(maxColumnIndex, depth);
    });

    nextComponentTop += Math.max(nodeHeight, (maxRows - 1) * rowGap + nodeHeight) + componentGap;
  });

  const width = Math.max(900, left * 2 + maxColumnIndex * columnGap + nodeWidth);
  const height = Math.max(320, nextComponentTop + top - componentGap);
  const drawableEdges = visibleEdges
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
    const wheelDelta = Math.max(
      -CALL_GRAPH_MAX_WHEEL_DELTA,
      Math.min(CALL_GRAPH_MAX_WHEEL_DELTA, event.deltaY),
    );
    const factor = Math.exp(-wheelDelta * CALL_GRAPH_WHEEL_ZOOM_RATE);

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
    <div className="relative">
      <div className="absolute right-2 top-2 z-10 flex items-center gap-1">
        <div className="mr-1 rounded bg-white/85 px-1.5 py-0.5 font-mono text-xs text-slate-700 shadow-sm">
          {Math.round(viewport.scale * 100)}%
        </div>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7 bg-white/90"
          title="Zoom out"
          aria-label="Zoom out call graph"
          onClick={() => zoomAround(width / 2, height / 2, 1 / CALL_GRAPH_BUTTON_ZOOM_STEP)}
        >
          <ZoomOut />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7 bg-white/90"
          title="Zoom in"
          aria-label="Zoom in call graph"
          onClick={() => zoomAround(width / 2, height / 2, CALL_GRAPH_BUTTON_ZOOM_STEP)}
        >
          <ZoomIn />
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="h-7 w-7 bg-white/90"
          title="Reset view"
          aria-label="Reset call graph view"
          onClick={() => setViewport(INITIAL_CALL_GRAPH_VIEWPORT)}
        >
          <RotateCcw />
        </Button>
      </div>
      <div ref={wheelTargetRef} className="h-[32rem] overscroll-contain overflow-hidden bg-white">
        <svg
          className={`h-full w-full select-none touch-none ${isPanning ? "cursor-grabbing" : "cursor-grab"}`}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="Profile call graph"
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
            {drawableEdges.map((edge) => {
              const from = positions.get(edge.from)!;
              const to = positions.get(edge.to)!;
              const forward = to.x > from.x;
              const sameColumn = to.x === from.x;
              const startX = forward ? from.x + nodeWidth : from.x;
              const startY = from.y + nodeHeight / 2;
              const endX = forward ? to.x - 6 : to.x + nodeWidth + 6;
              const endY = to.y + nodeHeight / 2;
              const bend = forward ? Math.max(58, (endX - startX) / 2) : 70;
              const isHotPath = hotPathEdgeIDs.has(edge.id);
              const strokeWidth = isHotPath ? 2.5 + (edge.value / maxEdgeValue) * 2 : 0.9 + (edge.value / maxEdgeValue) * 2.5;
              const edgePath = forward
                ? `M ${startX} ${startY} C ${startX + bend} ${startY}, ${endX - bend} ${endY}, ${endX} ${endY}`
                : sameColumn
                  ? `M ${startX + nodeWidth} ${startY} C ${startX + nodeWidth + bend} ${startY}, ${endX + bend} ${endY}, ${endX} ${endY}`
                  : `M ${startX} ${startY} C ${startX - bend} ${startY}, ${endX + bend} ${endY}, ${endX} ${endY}`;

              return (
                <path
                  key={edge.id}
                  d={edgePath}
                  fill="none"
                  stroke={isHotPath ? "#475569" : "#94a3b8"}
                  strokeOpacity={isHotPath ? 0.78 : 0.16 + (edge.value / maxEdgeValue) * 0.42}
                  strokeWidth={strokeWidth}
                  strokeDasharray={edge.indirect ? "5 4" : undefined}
                  markerEnd="url(#call-arrow)"
                >
                  <title>
                    {edge.from} {"->"} {edge.to}: {formatProfileValue(edge.value, valueInfo)}
                    {edge.indirect ? `, bridged over ${edge.skippedFrames} hidden frames` : ""}
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
                    {valueInfo.label} {formatProfileValue(node.value, valueInfo)}
                  </text>
                  <title>
                    {node.label}: {formatProfileValue(node.value, valueInfo)}
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
    <div className="grid gap-3 lg:grid-cols-2">
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
    <section className="min-w-0 border-b border-slate-300 pb-3 last:border-b-0 last:pb-0 lg:border-b-0 lg:border-r lg:pb-0 lg:pr-3 lg:last:border-r-0 lg:last:pr-0">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="inline-flex items-center gap-2 text-sm font-semibold text-slate-950">
          <span className="h-2 w-5 rounded-full" style={{ backgroundColor: color }} />
          {title}
        </div>
        <div className="font-mono text-sm font-semibold text-slate-800">{latestPoint?.formattedValue ?? "-"}</div>
      </div>
      <div className="grid gap-3 xl:grid-cols-2">
        <TrendSegmentChart
          title="Last 60 minutes"
          detail={`${minutePoints.length}/${MAX_MINUTE_SAMPLES} per-minute averages`}
          points={minutePoints}
          color={color}
          maxValue={chartMax}
        />
        <TrendSegmentChart
          title="Last minute"
          detail={`${secondPoints.length}/${MAX_SECOND_SAMPLES} per-second samples`}
          points={secondPoints}
          color={color}
          maxValue={chartMax}
          includeSeconds
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
  const height = 74;
  const chartLeft = 34;
  const chartRight = 10;
  const chartTop = 18;
  const chartHeight = 34;
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
      <svg className="h-[4.5rem] w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${title} resource trend`}>
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
