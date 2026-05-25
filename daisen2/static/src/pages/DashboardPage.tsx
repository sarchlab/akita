import { useEffect, useMemo, useRef, useState } from "react";
import { RotateCcw } from "lucide-react";
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
  const [viewRange, setViewRange] = useState<TimeRange>({ startTime, endTime });
  const dataRange = useDebouncedValue(viewRange, DATA_RANGE_DEBOUNCE_MS);
  const dataPending = viewRange.startTime !== dataRange.startTime || viewRange.endTime !== dataRange.endTime;
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(0);
  const [primaryAxis, setPrimaryAxis] = useState("ReqInCount");
  const [secondaryAxis, setSecondaryAxis] = useState("AvgLatency");
  const { ref, size } = useElementSize<HTMLDivElement>();

  useEffect(() => {
    setViewRange({ startTime, endTime });
  }, [startTime, endTime]);

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
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-muted-foreground">Filter</span>
          <Input
            className="w-56"
            value={filter}
            placeholder="Component Name"
            onChange={(event) => {
              setFilter(event.target.value);
              setPage(0);
            }}
          />
        </div>
        <div className="flex min-w-72 items-center gap-2">
          <span className="flex items-center gap-1 text-sm font-medium">
            <span className="h-2.5 w-2.5 rounded-full bg-[#d7191c]" />
            Primary Y-Axis
          </span>
          <Select value={primaryAxis} onValueChange={setPrimaryAxis}>
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
          <Select value={secondaryAxis} onValueChange={setSecondaryAxis}>
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
              />
            ))}
          </div>
        )}
      </div>

      <div className="flex h-14 shrink-0 items-center justify-center gap-4 border-t bg-white px-4 text-sm">
        <Button type="button" variant="outline" size="sm" disabled={currentPage <= 0} onClick={() => setPage((value) => value - 1)}>
          Previous
        </Button>
        <span className="text-primary">
          Page {currentPage + 1} of {pageCount} ({filteredNames.length} components)
        </span>
        <Button type="button" variant="outline" size="sm" disabled={currentPage + 1 >= pageCount} onClick={() => setPage((value) => value + 1)}>
          Next
        </Button>
      </div>
    </div>
  );
}
