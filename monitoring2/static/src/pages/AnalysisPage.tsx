import { useCallback, useEffect, useMemo, useState } from "react";
import { Gauge, RefreshCcw } from "lucide-react";
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
    fetch(`/api/hangdetector/buffers?sort=${sortMethod}&limit=64`)
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

  return { buffers, refresh };
}

function bufferPercent(buffer: BufferState) {
  if (!buffer.cap) {
    return 0;
  }

  return Math.min(1, Math.max(0, buffer.level / buffer.cap));
}

export default function AnalysisPage() {
  const [sortMethod, setSortMethod] = useState<"percent" | "level">("percent");
  const { buffers, refresh } = useBuffers(sortMethod);

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
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <Gauge className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Analysis</h1>
            <div className="text-xs text-muted-foreground">
              {buffers.length} buffers, {totals.level}/{totals.cap} slots occupied
            </div>
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
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            <RefreshCcw /> Refresh
          </Button>
        </header>

        <section className="rounded border bg-white p-4">
          <div className="mb-2 flex items-center justify-between text-sm">
            <span className="font-semibold">Aggregate Buffer Level</span>
            <span className="font-mono text-xs text-muted-foreground">
              {totals.level}/{totals.cap}
            </span>
          </div>
          <div className="h-3 overflow-hidden rounded-full bg-slate-200">
            <div className="h-full bg-amber-500" style={{ width: `${totalPercent * 100}%` }} />
          </div>
        </section>

        <section className="overflow-hidden rounded border bg-white">
          {buffers.length ? (
            buffers.map((buffer) => {
              const percent = bufferPercent(buffer);
              return (
                <div key={buffer.buffer} className="border-b p-4 last:border-b-0">
                  <div className="mb-2 flex items-center justify-between gap-3 text-sm">
                    <div className="min-w-0 truncate font-semibold">{buffer.buffer}</div>
                    <div className="shrink-0 font-mono text-xs text-muted-foreground">
                      {buffer.level}/{buffer.cap}
                    </div>
                  </div>
                  <div className="h-3 overflow-hidden rounded-full bg-slate-200">
                    <div className="h-full bg-amber-500" style={{ width: `${percent * 100}%` }} />
                  </div>
                  <div className="mt-2 grid grid-cols-3 gap-3 text-xs text-muted-foreground">
                    <span>Level {buffer.level}</span>
                    <span>Capacity {buffer.cap}</span>
                    <span>{Math.round(percent * 100)}%</span>
                  </div>
                </div>
              );
            })
          ) : (
            <div className="p-10 text-center text-sm text-muted-foreground">No buffers reported.</div>
          )}
        </section>
      </div>
    </div>
  );
}
