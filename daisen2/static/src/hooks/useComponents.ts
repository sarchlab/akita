import { useEffect, useState } from "react";
import type { ComponentResidency } from "../types/overview";
import { useRenderReady } from "./useRenderReady";

/** Fetches components ranked by total task time from /api/components. */
export function useComponents() {
  const [data, setData] = useState<ComponentResidency[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/components")
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: ComponentResidency[]) => {
        if (!cancelled) setData(json);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useRenderReady(loading, error !== null);

  return { data, loading, error };
}
