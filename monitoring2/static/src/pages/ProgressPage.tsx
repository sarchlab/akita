import { useCallback, useEffect, useMemo, useState } from "react";
import { ListChecks, Play, RefreshCcw, Square } from "lucide-react";
import { Button } from "../components/ui/button";

interface ProgressBarState {
  id: number;
  name: string;
  total: number;
  finished: number;
  in_progress: number;
}

interface TraceStorageState {
  path: string;
  file_size_bytes: number;
  sidecar_size_bytes: number;
  total_size_bytes: number;
  disk_available_bytes: number;
  disk_total_bytes: number;
}

function isProgressBarState(value: unknown): value is ProgressBarState {
  if (!value || typeof value !== "object") {
    return false;
  }

  const progress = value as Partial<ProgressBarState>;
  return (
    typeof progress.id === "number" &&
    typeof progress.name === "string" &&
    typeof progress.total === "number" &&
    typeof progress.finished === "number" &&
    typeof progress.in_progress === "number"
  );
}

function isTraceStorageState(value: unknown): value is TraceStorageState {
  if (!value || typeof value !== "object") {
    return false;
  }

  const storage = value as Partial<TraceStorageState>;
  return (
    typeof storage.path === "string" &&
    typeof storage.file_size_bytes === "number" &&
    typeof storage.sidecar_size_bytes === "number" &&
    typeof storage.total_size_bytes === "number" &&
    typeof storage.disk_available_bytes === "number" &&
    typeof storage.disk_total_bytes === "number"
  );
}

function useProgressBars() {
  const [progressBars, setProgressBars] = useState<ProgressBarState[]>([]);

  const refresh = useCallback(() => {
    fetch("/api/progress")
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setProgressBars(Array.isArray(json) ? json.filter(isProgressBarState) : []);
      })
      .catch(() => setProgressBars([]));
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { progressBars, refresh };
}

function useTraceStatus() {
  const [isTracing, setIsTracing] = useState(false);

  const refresh = useCallback(() => {
    fetch("/api/trace/is_tracing")
      .then((response) => (response.ok ? response.json() : null))
      .then((json: unknown) => {
        if (json && typeof json === "object" && "isTracing" in json) {
          setIsTracing(Boolean((json as { isTracing: unknown }).isTracing));
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { isTracing, refresh };
}

function useTraceStorage() {
  const [storage, setStorage] = useState<TraceStorageState | null>(null);

  const refresh = useCallback(() => {
    fetch("/api/trace/storage")
      .then((response) => (response.ok ? response.json() : null))
      .then((json: unknown) => {
        setStorage(isTraceStorageState(json) ? json : null);
      })
      .catch(() => setStorage(null));
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { storage, refresh };
}

async function post(path: string) {
  const response = await fetch(path, { method: "POST" });
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }
}

function clampPercent(value: number) {
  return Math.min(1, Math.max(0, value));
}

function finishedPercent(progress: ProgressBarState) {
  if (!progress.total) {
    return 0;
  }

  return clampPercent(progress.finished / progress.total);
}

function activePercent(progress: ProgressBarState) {
  if (!progress.total) {
    return 0;
  }

  return clampPercent((progress.finished + progress.in_progress) / progress.total);
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes)) {
    return "-";
  }

  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = bytes;
  let unit = 0;

  while (Math.abs(value) >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }

  return `${value.toLocaleString(undefined, {
    maximumFractionDigits: unit === 0 ? 0 : 1,
  })} ${units[unit]}`;
}

export default function ProgressPage() {
  const { progressBars, refresh } = useProgressBars();
  const { isTracing, refresh: refreshTraceStatus } = useTraceStatus();
  const { storage, refresh: refreshTraceStorage } = useTraceStorage();
  const [traceStatus, setTraceStatus] = useState("");
  const traceActionLabel = isTracing ? "Stop tracing" : "Start tracing";
  const TraceActionIcon = isTracing ? Square : Play;
  const sqliteBytes = storage?.total_size_bytes ?? storage?.file_size_bytes;

  const totals = useMemo(
    () =>
      progressBars.reduce(
        (acc, progress) => ({
          total: acc.total + progress.total,
          finished: acc.finished + progress.finished,
          inProgress: acc.inProgress + progress.in_progress,
        }),
        { total: 0, finished: 0, inProgress: 0 },
      ),
    [progressBars],
  );

  const runTraceAction = async (label: string, action: () => Promise<void>) => {
    setTraceStatus(`${label}...`);
    try {
      await action();
      setTraceStatus(`${label} complete`);
    } catch (err) {
      setTraceStatus(err instanceof Error ? err.message : `${label} failed`);
    }
  };

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <ListChecks className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Execution</h1>
            <div className="text-xs text-muted-foreground">
              {progressBars.length} bars, {totals.finished}/{totals.total} finished, {totals.inProgress} in progress
            </div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            <RefreshCcw /> Refresh
          </Button>
        </header>

        <section className="rounded border bg-white p-4">
          <div className="flex flex-wrap items-center gap-3">
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold">Tracing</div>
              <div className="mt-1 text-xs text-muted-foreground">
                Status: <span className="font-semibold text-foreground">{isTracing ? "on" : "off"}</span>
                {traceStatus ? <span className="ml-3">{traceStatus}</span> : null}
              </div>
            </div>
            <Button
              type="button"
              size="sm"
              variant={isTracing ? "outline" : "default"}
              onClick={() =>
                runTraceAction(traceActionLabel, () =>
                  post(isTracing ? "/api/trace/end" : "/api/trace/start").then(() => {
                    refreshTraceStatus();
                    refreshTraceStorage();
                  }),
                )
              }
            >
              <TraceActionIcon /> {isTracing ? "Stop Tracing" : "Start Tracing"}
            </Button>
          </div>
          <div className="mt-4 grid gap-4 border-t pt-3 sm:grid-cols-2">
            <div className="min-w-0">
              <div className="text-xs font-medium text-muted-foreground">SQLite file</div>
              <div className="mt-1 font-mono text-lg font-semibold">
                {sqliteBytes == null ? "-" : formatBytes(sqliteBytes)}
              </div>
              {storage && storage.sidecar_size_bytes > 0 ? (
                <div className="mt-1 text-[11px] text-muted-foreground">
                  Main {formatBytes(storage.file_size_bytes)}, WAL/SHM {formatBytes(storage.sidecar_size_bytes)}
                </div>
              ) : null}
              <div className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={storage?.path ?? ""}>
                {storage?.path ?? "No trace database reported"}
              </div>
            </div>
            <div>
              <div className="text-xs font-medium text-muted-foreground">Available disk</div>
              <div className="mt-1 font-mono text-lg font-semibold">
                {storage ? formatBytes(storage.disk_available_bytes) : "-"}
              </div>
              <div className="mt-1 text-[11px] text-muted-foreground">
                {storage ? `${formatBytes(storage.disk_total_bytes)} total` : "Filesystem unavailable"}
              </div>
            </div>
          </div>
        </section>

        <section className="overflow-hidden rounded border bg-white">
          {progressBars.length ? (
            progressBars.map((progress) => {
              const finished = finishedPercent(progress);
              const active = activePercent(progress);
              const completedShare = active ? finished / active : 0;

              return (
                <div key={progress.id} className="border-b p-4 last:border-b-0">
                  <div className="mb-2 flex items-center justify-between gap-3 text-sm">
                    <div className="min-w-0 truncate font-semibold">{progress.name}</div>
                    <div className="shrink-0 font-mono text-xs text-muted-foreground">
                      {progress.finished}/{progress.total}
                    </div>
                  </div>
                  <div className="h-3 overflow-hidden rounded-full bg-slate-200">
                    <div className="h-full bg-sky-300" style={{ width: `${active * 100}%` }}>
                      <div className="h-full bg-sky-600" style={{ width: `${completedShare * 100}%` }} />
                    </div>
                  </div>
                  <div className="mt-2 grid grid-cols-3 gap-3 text-xs text-muted-foreground">
                    <span>Finished {progress.finished}</span>
                    <span>In progress {progress.in_progress}</span>
                    <span>Total {progress.total}</span>
                  </div>
                </div>
              );
            })
          ) : (
            <div className="p-10 text-center text-sm text-muted-foreground">No progress bars reported.</div>
          )}
        </section>
      </div>
    </div>
  );
}
