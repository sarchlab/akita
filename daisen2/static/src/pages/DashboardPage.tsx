import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { RotateCcw, X } from "lucide-react";
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
import { parseView, mergeParams, DASHBOARD_DEFAULTS } from "../utils/viewState.mjs";

const AXIS_OPTIONS = [
  { value: "ReqInCount", label: "Incoming Request Rate" },
  { value: "ReqCompleteCount", label: "Request Complete Rate" },
  { value: "AvgLatency", label: "Average Request Latency" },
  { value: "ConcurrentTask", label: "Number Concurrent Task" },
  { value: "BufferPressure", label: "Buffer Pressure" },
  { value: "PendingReqOut", label: "Pending Request Out" },
  { value: "-", label: " - " },
];

const DATA_RANGE_DEBOUNCE_MS = 1000;

interface TimeRange {
  startTime: number;
  endTime: number;
}

function useElementSize<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  const [size, setSize] = useState({ width: 1200, height: 720 });

  useEffect(() => {
    if (!ref.current) return;
    const observer = new ResizeObserver(([entry]) => {
      setSize({
        width: entry.contentRect.width,
        height: entry.contentRect.height,
      });
    });
    observer.observe(ref.current);
    return () => observer.disconnect();
  }, []);

  return { ref, size };
}

function useDebouncedValue<T>(value: T, delayMs: number) {
  const [debouncedValue, setDebouncedValue] = useState(value);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setDebouncedValue(value);
    }, delayMs);

    return () => window.clearTimeout(timeout);
  }, [delayMs, value]);

  return debouncedValue;
}

export default function DashboardPage() {
  const { names, loading, error } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segmentsData } = useSegments();
  const [searchParams, setSearchParams] = useSearchParams();

  // The URL is the source of truth for the discrete view fields.
  const view = parseView("/dashboard", searchParams);
  const filter = view.filter ?? "";
  const page = view.page ?? 0;
  const primaryAxis = view.primary ?? DASHBOARD_DEFAULTS.primary;
  const secondaryAxis = view.secondary ?? DASHBOARD_DEFAULTS.secondary;
  const widget = view.widget ?? "";

  const patchView = (patch: Record<string, string | number | undefined>) => {
    setSearchParams((prev) => mergeParams("/dashboard", prev, patch), { replace: true });
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

  // Mirror the (debounced) range into the URL, omitting it when it equals the
  // simulation range so a fresh dashboard URL stays "/dashboard".
  useEffect(() => {
    // "Not zoomed" counts as at-sim even while the debounced dataRange is still
    // catching up to a just-loaded sim range, so the URL never transiently carries
    // the pre-load default (0..1e-6) range.
    const atSim =
      (dataRange.startTime === startTime && dataRange.endTime === endTime) ||
      (viewRange.startTime === startTime && viewRange.endTime === endTime);
    setSearchParams(
      (prev) => {
        const next = mergeParams("/dashboard", prev, {
          startTime: atSim ? undefined : dataRange.startTime,
          endTime: atSim ? undefined : dataRange.endTime,
        });
        // No-op when nothing changed, so this can never churn history or re-trigger.
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

  const { ref, size } = useElementSize<HTMLDivElement>();

  const columns = size.width >= 1500 ? 4 : size.width >= 1000 ? 3 : 2;
  const rows = 4;
  const widgetsPerPage = columns * rows;
  const widgetWidth = Math.max(180, Math.floor((size.width - (columns + 1) * 5) / columns));
  const widgetHeight = Math.max(120, Math.floor((size.height - 58 - (rows + 1) * 5) / rows));

  const filteredNames = useMemo(() => {
    if (!filter) return names;
    try {
      const regex = new RegExp(filter);
      return names.filter((name) => regex.test(name));
    } catch {
      return names.filter((name) => name.includes(filter));
    }
  }, [filter, names]);

  const totalWidgetCount = filteredNames.length;
  const pageCount = Math.max(1, Math.ceil(totalWidgetCount / widgetsPerPage));
  const currentPage = Math.min(page, pageCount - 1);
  const pageStart = currentPage * widgetsPerPage;
  const pageEnd = pageStart + widgetsPerPage;
  const visibleNames = filteredNames.slice(pageStart, pageEnd);

  const singleWidget = widget !== "";

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <form
        className="flex min-h-12 flex-wrap items-center gap-3 border-b bg-white px-4 py-2"
        onSubmit={(event) => event.preventDefault()}
      >
        <Button type="button" onClick={() => setViewRange({ startTime, endTime })}>
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
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-muted-foreground">Filter</span>
            <Input
              className="w-56"
              value={filter}
              placeholder="Component Name"
              onChange={(event) => patchView({ filter: event.target.value || undefined, page: undefined })}
            />
          </div>
        )}
        <div className="flex min-w-72 items-center gap-2">
          <span className="flex items-center gap-1 text-sm font-medium">
            <span className="h-2.5 w-2.5 rounded-full bg-[#d7191c]" />
            Primary Y-Axis
          </span>
          <Select value={primaryAxis} onValueChange={(value) => patchView({ primary: value })}>
            <SelectTrigger className="w-56">
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
        <div className="flex min-w-72 items-center gap-2">
          <span className="flex items-center gap-1 text-sm font-medium">
            <span className="h-2.5 w-2.5 rounded-full bg-[#2c7bb6]" />
            Secondary Y-Axis
          </span>
          <Select value={secondaryAxis} onValueChange={(value) => patchView({ secondary: value })}>
            <SelectTrigger className="w-56">
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
      </form>

      <div ref={ref} className="min-h-0 flex-1 overflow-hidden bg-white p-[5px]">
        {loading ? (
          <div className="flex h-full items-center justify-center text-muted-foreground">Loading components...</div>
        ) : error ? (
          <div className="flex h-full items-center justify-center text-destructive">{error}</div>
        ) : singleWidget ? (
          <DashboardWidget
            key={widget}
            name={widget}
            width={Math.max(180, size.width - 10)}
            height={Math.max(120, size.height - 10)}
            startTime={viewRange.startTime}
            endTime={viewRange.endTime}
            dataStartTime={dataRange.startTime}
            dataEndTime={dataRange.endTime}
            dataPending={dataPending}
            primaryAxis={primaryAxis}
            secondaryAxis={secondaryAxis}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            onTimeRangeChange={setViewRange}
          />
        ) : (
          <div
            className="daisen-dashboard-grid"
            style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: `${widgetHeight}px` }}
          >
            {visibleNames.map((name) => (
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
                onTimeRangeChange={setViewRange}
                onFocus={(focused) => patchView({ widget: focused })}
              />
            ))}
          </div>
        )}
      </div>

      {singleWidget ? null : (
        <div className="flex h-14 shrink-0 items-center justify-center gap-4 border-t bg-white px-4 text-sm">
          <Button type="button" variant="outline" size="sm" disabled={currentPage <= 0} onClick={() => patchView({ page: currentPage - 1 })}>
            Previous
          </Button>
          <span className="text-primary">
            Page {currentPage + 1} of {pageCount} ({filteredNames.length} components)
          </span>
          <Button type="button" variant="outline" size="sm" disabled={currentPage + 1 >= pageCount} onClick={() => patchView({ page: currentPage + 1 })}>
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
