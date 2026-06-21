import { useEffect, useState } from "react";
import type { Topology } from "../types/overview";
import { useRenderReady } from "./useRenderReady";

/** Fetches the simulation's component specs and port graph from /api/topology. */
export function useTopology() {
  const [data, setData] = useState<Topology | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch("/api/topology")
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: Topology) => {
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
