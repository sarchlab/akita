import { useEffect, useState, useCallback } from "react";
import type { Task } from "../types/task";

interface TraceQuery {
  id?: string;
  kind?: string;
  where?: string;
  parentId?: string;
  startTime?: number;
  endTime?: number;
}

interface TraceDataState {
  tasks: Task[];
  loading: boolean;
  error: string | null;
}

/**
 * Fetch trace data from /api/trace.
 *
 * Supports querying by id, kind/where, parentId, or time range.
 */
export function useTraceData(query: TraceQuery): TraceDataState & {
  refetch: (q?: TraceQuery) => void;
} {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchTasks = useCallback(
    (q: TraceQuery) => {
      const params = new URLSearchParams();
      if (q.id) params.set("id", q.id);
      if (q.kind) params.set("kind", q.kind);
      if (q.where) params.set("where", q.where);
      if (q.parentId) params.set("parentid", q.parentId);
      if (q.startTime != null) params.set("starttime", String(q.startTime));
      if (q.endTime != null) params.set("endtime", String(q.endTime));

      const qs = params.toString();
      if (!qs) {
        // No query → nothing to fetch
        return;
      }

      setLoading(true);
      setError(null);

      fetch(`/api/trace?${qs}`)
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`);
          return res.json();
        })
        .then((data: Task | Task[]) => {
          const arr = Array.isArray(data) ? data : [data];
          setTasks(arr.filter((t) => t && t.id));
          setLoading(false);
        })
        .catch((err: unknown) => {
          setError(err instanceof Error ? err.message : String(err));
          setLoading(false);
        });
    },
    [],
  );

  // Initial fetch
  useEffect(() => {
    fetchTasks(query);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query.id, query.kind, query.where, query.parentId, query.startTime, query.endTime]);

  return {
    tasks,
    loading,
    error,
    refetch: (q?: TraceQuery) => fetchTasks(q ?? query),
  };
}
