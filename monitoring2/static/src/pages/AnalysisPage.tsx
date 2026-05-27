import { useCallback, useEffect, useState } from "react";
import { X } from "lucide-react";
import { Button } from "../components/ui/button";
import {
  type PropertySample,
  usePropertyMonitoringSamples,
} from "../hooks/usePropertyMonitoringSamples";
import { formatPicosecondsAsNanoseconds } from "../utils/smartValue";
import {
  getWatchedProperties,
  removeWatchedProperty,
  subscribeToWatchedProperties,
  type WatchedProperty,
} from "../utils/watchedProperties";

interface BufferState {
  buffer: string;
  level: number;
  cap: number;
}

type AnalysisTab = "properties" | "buffers";
type BufferSortMethod = "name" | "level" | "fullness";

function isBufferState(value: unknown): value is BufferState {
  if (!value || typeof value !== "object") {
    return false;
  }

  const buffer = value as Partial<BufferState>;
  return (
    typeof buffer.buffer === "string" &&
    typeof buffer.level === "number" &&
    typeof buffer.cap === "number"
  );
}

function sortedBuffers(buffers: BufferState[], sortMethod: BufferSortMethod) {
  return [...buffers].sort((a, b) => {
    if (sortMethod === "name") {
      return a.buffer.localeCompare(b.buffer);
    }

    if (sortMethod === "level") {
      return b.level - a.level || a.buffer.localeCompare(b.buffer);
    }

    return bufferPercent(b) - bufferPercent(a) || a.buffer.localeCompare(b.buffer);
  });
}

function useBuffers(sortMethod: BufferSortMethod, enabled: boolean, autoRefresh: boolean) {
  const [buffers, setBuffers] = useState<BufferState[]>([]);

  const refresh = useCallback(() => {
    if (!enabled) {
      return;
    }

    const apiSortMethod = sortMethod === "level" ? "level" : "percent";

    fetch(`/api/hangdetector/buffers?sort=${apiSortMethod}&limit=256`)
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        const nextBuffers = Array.isArray(json) ? json.filter(isBufferState) : [];
        setBuffers(sortedBuffers(nextBuffers, sortMethod));
      })
      .catch(() => setBuffers([]));
  }, [enabled, sortMethod]);

  useEffect(() => {
    if (!enabled) {
      return;
    }

    refresh();
    if (!autoRefresh) {
      return;
    }

    const id = window.setInterval(refresh, 1500);
    return () => window.clearInterval(id);
  }, [autoRefresh, enabled, refresh]);

  return { buffers };
}

function bufferPercent(buffer: BufferState) {
  if (!buffer.cap) {
    return 0;
  }

  return Math.min(1, Math.max(0, buffer.level / buffer.cap));
}

function bufferFillClass(percent: number) {
  if (percent >= 0.9) {
    return "bg-red-500";
  }

  if (percent >= 0.7) {
    return "bg-amber-500";
  }

  return "bg-sky-600";
}

function useWatchedProperties() {
  const [properties, setProperties] = useState<WatchedProperty[]>(() => getWatchedProperties());

  useEffect(
    () =>
      subscribeToWatchedProperties(() => {
        setProperties(getWatchedProperties());
      }),
    [],
  );

  return properties;
}

function formatPropertyValue(value: number) {
  if (Math.abs(value) >= 1000 || (Math.abs(value) > 0 && Math.abs(value) < 0.01)) {
    return value.toExponential(2);
  }

  return Number.isInteger(value) ? String(value) : value.toFixed(2);
}

function PropertyChart({
  property,
  samples,
}: {
  property: WatchedProperty;
  samples: PropertySample[];
}) {
  const width = 280;
  const height = 76;
  const left = 30;
  const right = 8;
  const top = 10;
  const bottom = 20;
  const drawableWidth = width - left - right;
  const drawableHeight = height - top - bottom;
  const latest = samples[samples.length - 1];
  const minValue = samples.length ? Math.min(...samples.map((sample) => sample.value)) : 0;
  const maxValue = samples.length ? Math.max(...samples.map((sample) => sample.value)) : 1;
  const minTime = samples.length ? samples[0].timePs : 0;
  const maxTime = samples.length ? samples[samples.length - 1].timePs : 1;
  const valueRange = maxValue - minValue || 1;
  const timeRange = maxTime - minTime || 1;
  const points = samples.map((sample) => {
    const x = left + ((sample.timePs - minTime) / timeRange) * drawableWidth;
    const y = top + drawableHeight - ((sample.value - minValue) / valueRange) * drawableHeight;
    return { ...sample, x, y };
  });
  const path = points.map((point, index) => `${index === 0 ? "M" : "L"}${point.x},${point.y}`).join(" ");

  return (
    <div className="min-w-0 rounded border bg-white p-2">
      <div className="mb-1 flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-xs font-semibold">{property.field}</div>
          <div className="truncate text-[11px] text-muted-foreground">
            {property.component} - {property.sampleKind === "count" ? "element count" : "value"}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <div className="font-mono text-xs font-semibold">
            {latest ? formatPropertyValue(latest.value) : "-"}
          </div>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            title="Remove property"
            onClick={() => removeWatchedProperty(property.component, property.field)}
          >
            <X />
          </Button>
        </div>
      </div>
      {points.length ? (
        <svg viewBox={`0 0 ${width} ${height}`} className="h-[4.75rem] w-full overflow-visible">
          <line x1={left} y1={top + drawableHeight} x2={width - right} y2={top + drawableHeight} stroke="#94a3b8" />
          <line x1={left} y1={top} x2={left} y2={top + drawableHeight} stroke="#94a3b8" />
          <text x={left - 4} y={top + 4} textAnchor="end" fontSize="9" fill="#475569">
            {formatPropertyValue(maxValue)}
          </text>
          <text x={left - 4} y={top + drawableHeight} textAnchor="end" fontSize="9" fill="#475569">
            {formatPropertyValue(minValue)}
          </text>
          <text x={left} y={height - 3} fontSize="9" fill="#475569">
            {formatPicosecondsAsNanoseconds(minTime)}
          </text>
          <text x={width - right} y={height - 3} textAnchor="end" fontSize="9" fill="#475569">
            {formatPicosecondsAsNanoseconds(maxTime)}
          </text>
          <path d={path} fill="none" stroke="#0369a1" strokeWidth="2" />
          {points.map((point) => (
            <circle key={`${point.timePs}-${point.value}`} cx={point.x} cy={point.y} r="2" fill="#0369a1">
              <title>
                {formatPicosecondsAsNanoseconds(point.timePs)}: {formatPropertyValue(point.value)}
              </title>
            </circle>
          ))}
        </svg>
      ) : (
        <div className="flex h-[4.75rem] items-center justify-center text-xs text-muted-foreground">
          Waiting for numeric samples.
        </div>
      )}
    </div>
  );
}

function analysisTabButtonClass(active: boolean) {
  return active
    ? "border-primary bg-primary text-primary-foreground"
    : "border-input bg-white text-foreground hover:bg-slate-50";
}

export default function AnalysisPage() {
  const [activeTab, setActiveTab] = useState<AnalysisTab>("properties");
  const [bufferSortMethod, setBufferSortMethod] = useState<BufferSortMethod>("fullness");
  const [autoRefreshBuffers, setAutoRefreshBuffers] = useState(true);
  const { buffers } = useBuffers(bufferSortMethod, activeTab === "buffers", autoRefreshBuffers);
  const watchedProperties = useWatchedProperties();
  const propertySamples = usePropertyMonitoringSamples();

  return (
    <div className="h-full overflow-auto bg-slate-50 p-3">
      <div className="mx-auto flex max-w-[96rem] flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2" role="tablist" aria-label="Analysis views">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "properties"}
            className={`rounded border px-3 py-1.5 text-sm font-medium ${analysisTabButtonClass(
              activeTab === "properties",
            )}`}
            onClick={() => setActiveTab("properties")}
          >
            Property Monitoring
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "buffers"}
            className={`rounded border px-3 py-1.5 text-sm font-medium ${analysisTabButtonClass(
              activeTab === "buffers",
            )}`}
            onClick={() => setActiveTab("buffers")}
          >
            Buffer Level Analysis
          </button>
        </div>

        {activeTab === "properties" ? (
          <section>
            {watchedProperties.length ? (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-2">
                {watchedProperties.map((property) => (
                  <PropertyChart
                    key={property.id}
                    property={property}
                    samples={propertySamples[property.id] ?? []}
                  />
                ))}
              </div>
            ) : (
              <div className="rounded border bg-white p-10 text-center text-sm text-muted-foreground">
                No properties selected for monitoring.
              </div>
            )}
          </section>
        ) : (
          <section className="flex flex-col gap-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-sm font-semibold">Buffer Level Analysis</div>
              <div className="flex flex-wrap items-center gap-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium text-muted-foreground">Sort by</span>
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant={bufferSortMethod === "name" ? "default" : "outline"}
                      onClick={() => setBufferSortMethod("name")}
                    >
                      Name
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant={bufferSortMethod === "level" ? "default" : "outline"}
                      onClick={() => setBufferSortMethod("level")}
                    >
                      Buffer Level
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant={bufferSortMethod === "fullness" ? "default" : "outline"}
                      onClick={() => setBufferSortMethod("fullness")}
                    >
                      Fullness
                    </Button>
                  </div>
                </div>
                <label className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-primary"
                    checked={autoRefreshBuffers}
                    onChange={(event) => setAutoRefreshBuffers(event.target.checked)}
                  />
                  Auto refresh
                </label>
              </div>
            </div>
            {buffers.length ? (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-2">
                {buffers.map((buffer) => {
                  const percent = bufferPercent(buffer);
                  const percentLabel = Math.round(percent * 100);

                  return (
                    <div key={buffer.buffer} className="min-w-0 rounded border bg-white p-2">
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0 break-all text-xs font-semibold leading-4">{buffer.buffer}</div>
                        <div className="shrink-0 font-mono text-[11px] text-muted-foreground">{percentLabel}%</div>
                      </div>
                      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-200">
                        <div className={`h-full ${bufferFillClass(percent)}`} style={{ width: `${percent * 100}%` }} />
                      </div>
                      <div className="mt-1.5 flex items-center justify-between font-mono text-[11px] text-muted-foreground">
                        <span>level {buffer.level}</span>
                        <span>cap {buffer.cap}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="rounded border bg-white p-10 text-center text-sm text-muted-foreground">
                No buffers reported.
              </div>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
