import { useCallback, useEffect, useState } from "react";
import { Gauge, X } from "lucide-react";
import { Button } from "../components/ui/button";
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

interface SethNode {
  k: number;
  v?: unknown;
  l?: number;
}

interface SethSnapshot {
  r: string;
  dict: Record<string, SethNode>;
}

interface PropertySample {
  timePs: number;
  value: number;
}

type PropertySamples = Record<string, PropertySample[]>;

const MAX_PROPERTY_SAMPLES = 120;
const PROPERTY_SAMPLE_INTERVAL_MS = 1000;

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

function fieldRequestPath(componentName: string, fieldName: string) {
  return `/api/field/${encodeURIComponent(
    JSON.stringify({ comp_name: componentName, field_name: fieldName }),
  )}`;
}

function watchedSnapshotValue(snapshot: SethSnapshot, sampleKind: WatchedProperty["sampleKind"]) {
  const node = snapshot.dict[snapshot.r];
  if (!node) {
    return null;
  }

  if (sampleKind === "count") {
    if (node.k === 0) {
      return 0;
    }

    if (typeof node.l === "number" && Number.isFinite(node.l)) {
      return node.l;
    }

    if (Array.isArray(node.v)) {
      return node.v.length;
    }

    if (node.v && typeof node.v === "object") {
      return Object.keys(node.v).length;
    }

    return null;
  }

  const value = node.v;

  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  return null;
}

async function fetchEngineTime() {
  const response = await fetch("/api/now");
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  const json = (await response.json()) as { now?: unknown };
  return typeof json.now === "number" ? json.now : Date.now() * 1_000_000_000;
}

async function fetchWatchedPropertyValue(property: WatchedProperty) {
  const response = await fetch(fieldRequestPath(property.component, property.field));
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return watchedSnapshotValue((await response.json()) as SethSnapshot, property.sampleKind);
}

function useBuffers(sortMethod: "percent" | "level") {
  const [buffers, setBuffers] = useState<BufferState[]>([]);

  const refresh = useCallback(() => {
    fetch(`/api/hangdetector/buffers?sort=${sortMethod}&limit=256`)
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setBuffers(Array.isArray(json) ? json.filter(isBufferState) : []);
      })
      .catch(() => setBuffers([]));
  }, [sortMethod]);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1500);
    return () => window.clearInterval(id);
  }, [refresh]);

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

function usePropertySamples(properties: WatchedProperty[]) {
  const [samples, setSamples] = useState<PropertySamples>({});

  useEffect(() => {
    const propertyIDs = new Set(properties.map((property) => property.id));

    setSamples((previous) =>
      Object.fromEntries(Object.entries(previous).filter(([id]) => propertyIDs.has(id))),
    );

    if (!properties.length) {
      return;
    }

    let cancelled = false;

    const collect = async () => {
      try {
        const timePs = await fetchEngineTime();
        const values = await Promise.all(
          properties.map(async (property) => ({
            property,
            value: await fetchWatchedPropertyValue(property),
          })),
        );

        if (cancelled) {
          return;
        }

        setSamples((previous) => {
          const next = { ...previous };

          values.forEach(({ property, value }) => {
            if (value === null) {
              return;
            }

            next[property.id] = [...(next[property.id] ?? []), { timePs, value }].slice(
              -MAX_PROPERTY_SAMPLES,
            );
          });

          return next;
        });
      } catch {
        // Keep existing samples if a component is temporarily unavailable.
      }
    };

    collect();
    const intervalID = window.setInterval(collect, PROPERTY_SAMPLE_INTERVAL_MS);

    return () => {
      cancelled = true;
      window.clearInterval(intervalID);
    };
  }, [properties]);

  return samples;
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

export default function AnalysisPage() {
  const [sortMethod, setSortMethod] = useState<"percent" | "level">("percent");
  const { buffers } = useBuffers(sortMethod);
  const watchedProperties = useWatchedProperties();
  const propertySamples = usePropertySamples(watchedProperties);

  return (
    <div className="h-full overflow-auto bg-slate-50 p-3">
      <div className="mx-auto flex max-w-[96rem] flex-col gap-3">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <Gauge className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Analysis</h1>
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant={sortMethod === "percent" ? "default" : "outline"}
              onClick={() => setSortMethod("percent")}
            >
              Percent
            </Button>
            <Button
              type="button"
              size="sm"
              variant={sortMethod === "level" ? "default" : "outline"}
              onClick={() => setSortMethod("level")}
            >
              Level
            </Button>
          </div>
        </header>

        {watchedProperties.length ? (
          <section className="grid grid-cols-[repeat(auto-fill,minmax(17rem,1fr))] gap-2">
            {watchedProperties.map((property) => (
              <PropertyChart
                key={property.id}
                property={property}
                samples={propertySamples[property.id] ?? []}
              />
            ))}
          </section>
        ) : null}

        <section>
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
      </div>
    </div>
  );
}
