import { useEffect, useState } from "react";
import type { DBInfo, DBInfoResponse } from "../types/db";
import { useRenderReady } from "./useRenderReady";

/**
 * Fetches the trace database's schema-and-size overview from /api/db_info. The
 * server computes the (expensive) dbstat scan once in the background, so while
 * `computing` is true this re-polls until the sizes are ready.
 */
export function useDBInfo() {
  const [data, setData] = useState<DBInfo | null>(null);
  const [computing, setComputing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const poll = () => {
      fetch("/api/db_info")
        .then((response) => {
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          return response.json();
        })
        .then((json: DBInfoResponse) => {
          if (cancelled) return;
          setComputing(json.computing);
          if (json.info) setData(json.info);
          setLoading(false);
          if (json.computing) timer = setTimeout(poll, 1500);
        })
        .catch((err: unknown) => {
          if (cancelled) return;
          setError(err instanceof Error ? err.message : String(err));
          setLoading(false);
        });
    };

    poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, []);

  useRenderReady(loading, error !== null);

  return { data, computing, loading, error };
}
