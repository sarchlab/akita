import { useEffect, useRef, useState } from "react";
import { useMode } from "../../hooks/useMode";

/** Shape returned by GET /api/progress */
interface ProgressBar {
  id: string;
  name: string;
  start_time: string;
  total: number;
  in_progress: number;
  finished: number;
}

/** Max bars before switching to pie mode. */
const MAX_BAR_MODE = 2;

/**
 * ProgressBars — polls /api/progress every second and renders
 * Bootstrap progress bars (≤2 items) or pie charts (>2 items).
 * Only renders when the simulation mode is "live".
 */
export default function ProgressBars() {
  const { mode } = useMode();
  const [bars, setBars] = useState<ProgressBar[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    if (mode !== "live") return;

    const controller = new AbortController();

    const poll = () => {
      fetch("/api/progress", { signal: controller.signal })
        .then((res) => res.json())
        .then((data: ProgressBar[] | null) => {
          setBars(data ?? []);
        })
        .catch((err: unknown) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
        });
    };

    poll();
    timerRef.current = setInterval(poll, 1000);

    return () => {
      controller.abort();
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [mode]);

  if (mode !== "live" || bars.length === 0) return null;

  const usePie = bars.length > MAX_BAR_MODE;

  return (
    <div className="mb-3">
      <h6 className="mb-2">Progress</h6>
      {usePie ? (
        <div className="d-flex flex-wrap gap-3">
          {bars.map((b) => (
            <PieItem key={b.id} bar={b} />
          ))}
        </div>
      ) : (
        <div className="d-flex flex-column gap-2">
          {bars.map((b) => (
            <BarItem key={b.id} bar={b} />
          ))}
        </div>
      )}
    </div>
  );
}

/* ── Bar mode ─────────────────────────────────────────────── */

function BarItem({ bar }: { bar: ProgressBar }) {
  const finished = bar.finished ?? 0;
  const inProgress = bar.in_progress;
  const total = bar.total || 1; // avoid div-by-zero
  const unfinished = total - inProgress - finished;

  const pctFinished = (finished / total) * 100;
  const pctInProgress = (inProgress / total) * 100;
  const pctUnfinished = (unfinished / total) * 100;

  return (
    <div className="d-flex align-items-center gap-2">
      <label className="mb-0 text-nowrap" style={{ minWidth: 100 }}>
        {bar.name}
      </label>
      <div className="progress flex-grow-1" style={{ height: 20 }}>
        <div
          className="progress-bar bg-success"
          style={{ width: `${pctFinished}%` }}
        >
          {finished > 0 ? finished : ""}
        </div>
        <div
          className="progress-bar progress-bar-striped bg-primary"
          style={{ width: `${pctInProgress}%` }}
        >
          {inProgress > 0 ? inProgress : ""}
        </div>
        <div
          className="progress-bar bg-secondary"
          style={{ width: `${pctUnfinished}%` }}
        >
          {unfinished > 0 ? unfinished : ""}
        </div>
      </div>
      <small className="text-muted text-nowrap">
        {finished}/{total}
      </small>
    </div>
  );
}

/* ── Pie mode ─────────────────────────────────────────────── */

function PieItem({ bar }: { bar: ProgressBar }) {
  const finished = bar.finished ?? 0;
  const inProgress = bar.in_progress;
  const total = bar.total || 1;

  const finishedDeg = (finished / total) * 360;
  const inProgressDeg = (inProgress / total) * 360;

  const bg = `conic-gradient(
    #28a745 ${finishedDeg}deg,
    #007bff ${finishedDeg}deg ${finishedDeg + inProgressDeg}deg,
    #e9ecef ${finishedDeg + inProgressDeg}deg
  )`;

  return (
    <div className="text-center" style={{ width: 80 }} title={
      `${bar.name}\nFinished: ${finished}, In Progress: ${inProgress}, Total: ${total}`
    }>
      <div
        style={{
          width: 60,
          height: 60,
          borderRadius: "50%",
          background: bg,
          margin: "0 auto",
        }}
      />
      <small
        className="d-block text-truncate mt-1"
        style={{ fontSize: 11 }}
      >
        {bar.name}
      </small>
      <small className="text-muted" style={{ fontSize: 10 }}>
        {finished}/{total}
      </small>
    </div>
  );
}
