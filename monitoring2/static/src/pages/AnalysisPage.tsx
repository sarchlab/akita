import { useCallback, useEffect, useMemo, useState } from "react";
import { Gauge } from "lucide-react";
import { Button } from "../components/ui/button";

interface BufferState {
  buffer: string;
  level: number;
  cap: number;
}

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

export default function AnalysisPage() {
  const [sortMethod, setSortMethod] = useState<"percent" | "level">("percent");
  const { buffers } = useBuffers(sortMethod);

  const totals = useMemo(
    () =>
      buffers.reduce(
        (acc, buffer) => ({
          level: acc.level + buffer.level,
          cap: acc.cap + buffer.cap,
        }),
        { level: 0, cap: 0 },
      ),
    [buffers],
  );

  const totalPercent = totals.cap ? Math.min(1, totals.level / totals.cap) : 0;

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

        <section className="rounded border bg-white px-4 py-3">
          <div className="grid gap-3 sm:grid-cols-[11rem_minmax(0,1fr)_auto] sm:items-center">
            <div>
              <div className="text-sm font-semibold">Aggregate Buffer Level</div>
              <div className="mt-1 text-xs text-muted-foreground">{buffers.length} buffers</div>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-slate-200">
              <div className="h-full bg-amber-500" style={{ width: `${totalPercent * 100}%` }} />
            </div>
            <div className="font-mono text-xs text-muted-foreground">
              {totals.level}/{totals.cap}
            </div>
          </div>
        </section>

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
