import { useEffect, useRef, useState } from "react";
import { useRenderReady } from "./useRenderReady";

// A downsampled, level-of-detail view of a component's tasks: tasks binned by
// start time and grouped by "Kind-What" color key. Lets the page draw a density
// chart for busy components without fetching/rendering one element per task.
export interface ComponentTimelineData {
  start_time: number;
  end_time: number;
  num_bins: number;
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
    // if the new fetch is slow or fails). A range-only change keeps the previous data
    // for a smooth, flicker-free zoom.
    const key = `${scope}\n${group}`;
    if (lastKeyRef.current !== key) {
      lastKeyRef.current = key;
      setData(null);
    }

    if (!scope || !(endTime > startTime) || numBins < 1) {
      setData(null);
      return undefined;
    }

    const controller = new AbortController();
    const params = new URLSearchParams({
      scope,
      starttime: String(startTime),
      endtime: String(endTime),
      num_bins: String(numBins),
      group,
    });

    setLoading(true);
    setError(null);
    fetch(`/api/component_timeline?${params.toString()}`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((d: ComponentTimelineData) => setData(d))
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [scope, startTime, endTime, numBins, group]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
