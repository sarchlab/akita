import { useEffect, useState } from "react";
import type { SimInfoEntry } from "../types/overview";
import { useRenderReady } from "./useRenderReady";

/** Fetches the simulation's exec_info key/value rows from /api/sim_info. */
export function useSimInfo() {
  const [data, setData] = useState<SimInfoEntry[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/sim_info")
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: SimInfoEntry[]) => {
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
