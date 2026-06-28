import { useCallback, useEffect, useRef, useState } from "react";
import type { Task } from "../types/task";

// Auto-load descendant levels until this many descendants are loaded; past it,
// each further level waits for an explicit "Expand next level" click so a huge
// subtree doesn't load all at once.
const AUTO_EXPAND_LIMIT = 300;

interface TaskHierarchy {
  mainTask: Task | null;
  // Ancestors root-first: [root, …, immediate parent].
  ancestors: Task[];
  // Descendant levels: [children, grandchildren, …]; each entry is one level.
  levels: Task[][];
  // True once the deepest loaded level has no further children.
  atLeaves: boolean;
  loading: boolean;
  expanding: boolean;
  expandNext: () => void;
}

function normalize(raw: unknown): Task[] {
  return (raw as Array<Record<string, unknown>>).map((t) => ({
    ...(t as unknown as Task),
    id: String(t.id),
    parent_id: t.parent_id === 0 ? "" : String(t.parent_id),
  }));
}

async function fetchTrace(params: Record<string, string>, signal: AbortSignal): Promise<Task[]> {
  const response = await fetch(`/api/trace?${new URLSearchParams(params).toString()}`, { signal });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);

  return normalize(await response.json());
}

// useTaskHierarchy loads a task with its full ancestor chain and its descendant
// subtree. Descendant levels load top-down, automatically up to AUTO_EXPAND_LIMIT
// tasks and then one level per expandNext() call.
export function useTaskHierarchy(taskId: string): TaskHierarchy {
  const [mainTask, setMainTask] = useState<Task | null>(null);
  const [ancestors, setAncestors] = useState<Task[]>([]);
  const [levels, setLevels] = useState<Task[][]>([]);
  const [atLeaves, setAtLeaves] = useState(false);
  const [loading, setLoading] = useState(false);
  const [expanding, setExpanding] = useState(false);

  // Mutable fetch state, kept in refs so expandNext sees the latest without
  // re-subscribing: the ids of the deepest loaded level (parents of the next),
  // a generation token to drop stale results, and the active abort controller.
  const frontierIds = useRef<string[]>([]);
  const gen = useRef(0);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => {
    gen.current += 1;
    const myGen = gen.current;
    const ctrl = new AbortController();
    controller.current = ctrl;

    setMainTask(null);
    setAncestors([]);
    setLevels([]);
    setAtLeaves(false);
    frontierIds.current = [];

    if (!taskId) return () => ctrl.abort();

    setLoading(true);
    void (async () => {
      try {
        const [main] = await fetchTrace({ id: taskId }, ctrl.signal);
        if (myGen !== gen.current) return;
        if (!main) {
          setLoading(false);
          return;
        }
        setMainTask(main);

        // Ancestor chain, walking up parent_id (guarded against cycles).
        const chain: Task[] = [];
        const seen = new Set<string>([String(main.id)]);
        let pid = String(main.parent_id ?? "");
        while (pid && !seen.has(pid)) {
          seen.add(pid);
          const [parent] = await fetchTrace({ id: pid }, ctrl.signal);
          if (myGen !== gen.current) return;
          if (!parent) break;
          chain.push(parent);
          pid = String(parent.parent_id ?? "");
        }
        setAncestors(chain.reverse());

        // Descendant levels, auto-expanding up to the limit.
        const loaded: Task[][] = [];
        let frontier = [String(main.id)];
        let total = 0;
        for (;;) {
          const children = await fetchTrace({ parentids: frontier.join(",") }, ctrl.signal);
          if (myGen !== gen.current) return;
          if (children.length === 0) {
            setAtLeaves(true);
            break;
          }
          loaded.push(children);
          setLevels([...loaded]);
          total += children.length;
          frontier = children.map((task) => String(task.id));
          frontierIds.current = frontier;
          if (total >= AUTO_EXPAND_LIMIT) break;
        }
        setLoading(false);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (myGen === gen.current) setLoading(false);
      }
    })();

    return () => ctrl.abort();
  }, [taskId]);

  const expandNext = useCallback(() => {
    const ctrl = controller.current;
    if (!ctrl || atLeaves || expanding || frontierIds.current.length === 0) return;
    const myGen = gen.current;
    setExpanding(true);
    void (async () => {
      try {
        const children = await fetchTrace({ parentids: frontierIds.current.join(",") }, ctrl.signal);
        if (myGen !== gen.current) return;
        if (children.length === 0) {
          setAtLeaves(true);
        } else {
          setLevels((prev) => [...prev, children]);
          frontierIds.current = children.map((task) => String(task.id));
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
      } finally {
        if (myGen === gen.current) setExpanding(false);
      }
    })();
  }, [atLeaves, expanding]);

  return { mainTask, ancestors, levels, atLeaves, loading, expanding, expandNext };
}
