import { useEffect, useState } from "react";
import type { SegmentsResponse } from "../types/task";

interface SegmentsState {
  data: SegmentsResponse | null;
  loading: boolean;
  error: string | null;
}

/**
 * Fetch segment data from /api/segments.
 * The result is fetched once and cached in state.
 */
export function useSegments(): SegmentsState {
  const [data, setData] = useState<SegmentsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    fetch("/api/segments", { signal: controller.signal })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.json();
      })
      .then((json: SegmentsResponse) => {
        setData(json);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, []);

  return { data, loading, error };
}
