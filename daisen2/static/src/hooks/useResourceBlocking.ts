import { useEffect, useState } from "react";
import { useRenderReady } from "./useRenderReady";

export interface ResourceBlockingData {
  what: string;
  start_time: number;
  end_time: number;
  num_bins: number;
  // 1-in-N task stride the server used; >1 means the counts are estimates.
  sample: number;
  // Distinct tasks blocked on the resource within the queried window (drives the
  // density-vs-gantt choice).
  total: number;
  // Distinct tasks that ever block on it (whole trace), for context.
  total_all: number;
  // Per-bin count of tasks blocked on the resource.
  bins: number[];
}

// useResourceBlocking fetches the occupancy of tasks blocked on one hardware
// resource (the milestone `what`) over a time range — the shaded-area series the
// resource page draws.
export function useResourceBlocking(
  what: string,
  startTime: number,
  endTime: number,
  numBins = 200,
) {
  const [data, setData] = useState<ResourceBlockingData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!what || startTime >= endTime) {
      setData(null);
      return;
    }

    const controller = new AbortController();
    const params = new URLSearchParams({
      what,
      starttime: String(startTime),
      endtime: String(endTime),
      num_bins: String(numBins),
    });

    setLoading(true);
    setError(null);
    fetch(`/api/resource_blocking?${params.toString()}`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: ResourceBlockingData) => setData(json))
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [what, startTime, endTime, numBins]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
