import { useEffect, useState } from "react";
import type { DBInfo, DBInfoResponse } from "../types/db";
import { useRenderReady } from "./useRenderReady";

// How often to re-poll /api/db_info once the sizes have settled. A later lazy
// index build invalidates the server-side cache and enlarges the database; a
// slow background poll picks that up (the next poll sees computing=true again and
// the fast poll below refetches the new sizes). The endpoint is designed to be
// cheap to poll — it serves the cached overview without recomputing.
const SETTLED_POLL_MS = 5000;
const COMPUTING_POLL_MS = 1500;

/**
 * Fetches the trace database's schema-and-size overview from /api/db_info. The
 * server computes the (expensive) dbstat scan once in the background, so while
 * `computing` is true this re-polls quickly until the sizes are ready, then keeps
 * polling slowly so a later index build's new sizes are observed.
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
          timer = setTimeout(
            poll,
            json.computing ? COMPUTING_POLL_MS : SETTLED_POLL_MS,
          );
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
