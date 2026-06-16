import { useCallback, useEffect, useState } from "react";
import type { Task } from "../types/task";
import { useRenderReady } from "./useRenderReady";

export interface TraceQuery {
  id?: string;
  kind?: string;
  where?: string;
  parentId?: string;
  startTime?: number;
  endTime?: number;
}

function normalizeTask(task: Task): Task {
  return { ...task, id: String(task.id), parent_id: task.parent_id === 0 ? "" : String(task.parent_id) };
}

export function useTraceData(query: TraceQuery) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchTasks = useCallback((q: TraceQuery, signal?: AbortSignal) => {
    const params = new URLSearchParams();
    if (q.id) params.set("id", q.id);
    if (q.kind) params.set("kind", q.kind);
    if (q.where) params.set("where", q.where);
    if (q.parentId) params.set("parentid", q.parentId);
    if (q.startTime != null) params.set("starttime", String(q.startTime));
    if (q.endTime != null) params.set("endtime", String(q.endTime));
    const queryString = params.toString();
    if (!queryString) {
      setTasks([]);
      return Promise.resolve();
    }

    setLoading(true);
    setError(null);
    return fetch(`/api/trace?${queryString}`, { signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((data: Task | Task[]) => {
        const list = Array.isArray(data) ? data : [data];
        setTasks(list.filter(Boolean).map(normalizeTask));
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        // A superseded (aborted) request must not clear loading for the request
        // that replaced it — that would flip render-ready true while data is stale.
        if (!signal?.aborted) setLoading(false);
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void fetchTasks(query, controller.signal);
    return () => controller.abort();
  }, [fetchTasks, query.id, query.kind, query.where, query.parentId, query.startTime, query.endTime]);

  useRenderReady(loading, error !== null);

  return { tasks, loading, error, refetch: (q?: TraceQuery) => fetchTasks(q ?? query) };
}
