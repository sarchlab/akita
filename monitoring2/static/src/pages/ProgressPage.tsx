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

export default function ProgressPage() {
  const { progressBars, refresh } = useProgressBars();
  const { isTracing, refresh: refreshTraceStatus } = useTraceStatus();
  const [traceStatus, setTraceStatus] = useState("");
  const traceActionLabel = isTracing ? "Stop tracing" : "Start tracing";
  const TraceActionIcon = isTracing ? Square : Play;

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
                  post(isTracing ? "/api/trace/end" : "/api/trace/start").then(refreshTraceStatus),
                )
              }
            >
              <TraceActionIcon /> {isTracing ? "Stop Tracing" : "Start Tracing"}
            </Button>
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
