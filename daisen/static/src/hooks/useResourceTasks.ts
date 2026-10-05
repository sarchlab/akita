import { useEffect, useState } from "react";
import type { Task } from "../types/task";
import { useRenderReady } from "./useRenderReady";

// useResourceTasks fetches the (small set of) tasks blocked on one hardware
// resource, with their milestones — for the resource page's per-task gantt. Only
// runs when `enabled` (i.e. the resource's task set is small enough to draw).
export function useResourceTasks(
  what: string,
  startTime: number,
  endTime: number,
  enabled: boolean,
  limit = 200,
) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled || !what || startTime >= endTime) {
      setTasks([]);
      return;
    }

    const controller = new AbortController();
    const params = new URLSearchParams({
      what,
      starttime: String(startTime),
      endtime: String(endTime),
      limit: String(limit),
    });

    setLoading(true);
    setError(null);
    fetch(`/api/resource_tasks?${params.toString()}`, { signal: controller.signal })
      .then((response) => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
      })
      .then((json: Task[]) => setTasks(json ?? []))
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [what, startTime, endTime, enabled, limit]);

  useRenderReady(loading, error !== null);

  return { tasks, loading, error };
}
