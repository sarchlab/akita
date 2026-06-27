import { useEffect, useRef, useState } from "react";
import { useRenderReady } from "./useRenderReady";

// Below this estimated task count, finish with an exact (sample=1) pass: the
// scope is both cheap to count exactly and near the thresholds the page uses to
// switch to the per-task view, so the scaled estimate must not be trusted. Above
// it, the scope is far too dense for the per-task view and an exact pass would
// cost minutes. Comfortably above ComponentPage's RAW_TASK_THRESHOLD (5000) to
// absorb sampling error near that boundary.
const EXACT_BELOW = 50_000;

// A downsampled, level-of-detail view of a component's tasks: tasks binned by
// start time and grouped by "Kind-What" color key. Lets the page draw a density
// chart for busy components without fetching/rendering one element per task.
export interface ComponentTimelineData {
  start_time: number;
  end_time: number;
  num_bins: number;
  // 1-in-N task stride this view was computed with (1 = exact). While a coarse
  // preview refines toward exact, this drops toward 1.
  sample?: number;
  // Total tasks overlapping the range — how many the per-task view would render.
  total: number;
  // Distinct color keys, sorted; matches the column order of every bins row.
  keys: string[];
  // Dense num_bins-by-keys.length matrix of start counts.
  bins: number[][];
}

export function useComponentTimeline(
  scope: string,
  startTime: number,
  endTime: number,
  numBins: number,
  // "kind" | "kind-what" — how the server groups tasks into bands. Must match the
  // client's taskColorKey so a band's key resolves to the same color.
  group: string = "kind-what",
) {
  const [data, setData] = useState<ComponentTimelineData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const lastKeyRef = useRef(`${scope}\n${group}`);

  useEffect(() => {
    // Drop stale data when the scope OR the grouping (color mode) changes. A new
    // scope's summary is not comparable to the previous one's — a small old `total`
    // must not green-light a huge raw-task fetch for a dense new scope — and toggling
    // the color mode re-buckets the bands, so keeping the old grouping would leave the
    // count bands and legend out of sync with the task bars (or stuck on the old mode
    // if the new fetch is slow or fails). A range / bin-count change keeps the previous
    // chart on screen while the new one loads, so the view never flickers to blank
    // between progressive passes or when the measured width re-quantizes numBins.
    const cacheKey = `${scope}\n${group}`;
    if (lastKeyRef.current !== cacheKey) {
      lastKeyRef.current = cacheKey;
      setData(null);
    }

    if (!scope || !(endTime > startTime) || numBins < 1) {
      return undefined;
    }

    const controller = new AbortController();
    setLoading(true);
    setError(null);

    // Progressive sample: a coarse 1-in-N task sample paints fast, then one
    // denser pass sharpens the counts. numBins is already pixel-appropriate, so it
    // stays fixed. We stop blocking the "ready" signal after the first pass; the
    // refinement runs in the background and aborts if the scope/range changes.
    const schedule = [128, 8];
    let firstDone = false;
    let lastTotal = Infinity;

    const runPass = async (sample: number): Promise<boolean> => {
      const params = new URLSearchParams({
        scope,
        starttime: String(startTime),
        endtime: String(endTime),
        num_bins: String(numBins),
        group,
      });
      if (sample > 1) params.set("sample", String(sample));
      const response = await fetch(
        `/api/component_timeline?${params.toString()}`,
        { signal: controller.signal },
      );
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const d: ComponentTimelineData = await response.json();
      if (controller.signal.aborted) return false;
      setData(d);
      lastTotal = d.total;
      if (!firstDone) {
        firstDone = true;
        setLoading(false);
      }

      return true;
    };

    void (async () => {
      try {
        for (const sample of schedule) {
          if (!(await runPass(sample))) return;
        }
        // The sampled passes use a deterministic rowid stride, so a sparse or
        // modulo-skewed scope can return zero rows (chart stuck at 0), and any
        // small scope's scaled estimate is too coarse exactly where the page
        // switches to the per-task view (RAW_TASK_THRESHOLD). At this size an exact
        // pass is cheap, so finish with sample=1. Above EXACT_BELOW the scope is far
        // too dense for the per-task view and an exact pass would cost minutes, so
        // we keep the scaled estimate.
        if (lastTotal < EXACT_BELOW) {
          await runPass(1);
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      }
    })();

    return () => controller.abort();
  }, [scope, startTime, endTime, numBins, group]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
