import { useEffect, useState } from "react";
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
  where: string,
  startTime: number,
  endTime: number,
  numBins: number,
) {
  const [data, setData] = useState<ComponentTimelineData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!where || !(endTime > startTime) || numBins < 1) {
      setData(null);
      return undefined;
    }

    const controller = new AbortController();
    const params = new URLSearchParams({
      where,
      starttime: String(startTime),
      endtime: String(endTime),
      num_bins: String(numBins),
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
  }, [where, startTime, endTime, numBins]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
