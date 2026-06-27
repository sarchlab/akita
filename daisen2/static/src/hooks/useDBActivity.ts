import { useEffect, useState } from "react";
import type { DBActivity } from "../types/db";

/**
 * Polls /api/db_activity for the set of in-flight database operations (index
 * builds, heavy queries, the dbstat scan), so the UI can show what the database
 * is doing in real time. Best-effort: a failed poll keeps the last value and
 * retries on the next tick.
 */
export function useDBActivity(intervalMs = 1000): DBActivity[] {
  const [data, setData] = useState<DBActivity[]>([]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const poll = () => {
      fetch("/api/db_activity")
        .then((response) => (response.ok ? response.json() : []))
        .then((json: DBActivity[]) => {
          if (!cancelled) setData(Array.isArray(json) ? json : []);
        })
        .catch(() => {
          /* best-effort: keep last value */
        })
        .finally(() => {
          if (!cancelled) timer = setTimeout(poll, intervalMs);
        });
    };

    poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [intervalMs]);

  return data;
}
