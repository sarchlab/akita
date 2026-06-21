import { useEffect, useState } from "react";
import type { BlockedComponent } from "../types/overview";
import { useRenderReady } from "./useRenderReady";

/** Fetches components ranked by blocked time from /api/blocked. */
export function useBlocked() {
  const [data, setData] = useState<BlockedComponent[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/blocked")
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: BlockedComponent[]) => {
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
