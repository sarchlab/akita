import { useEffect, useState } from "react";
import { useRenderReady } from "./useRenderReady";

export interface BlockingResource {
  what: string;
  // Number of blocking events on this resource (the ranking metric).
  count: number;
  // Distinct tasks that blocked on it.
  task_count: number;
}

export interface TopBlockingResourcesData {
  resources: BlockingResource[];
}

// useTopBlockingResources fetches the hardware resources that blocked tasks the
// most (ranked by blocking-event count) over a location scope ("" = the whole
// trace). Modeled on the other overview hooks: abort on change, render-ready
// wiring, abort-aware loading.
export function useTopBlockingResources(scope = "", limit = 10) {
  const [data, setData] = useState<TopBlockingResourcesData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const params = new URLSearchParams({ scope, limit: String(limit) });

    setLoading(true);
    setError(null);
    fetch(`/api/top_blocking_resources?${params.toString()}`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: TopBlockingResourcesData) => setData(json))
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [scope, limit]);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
