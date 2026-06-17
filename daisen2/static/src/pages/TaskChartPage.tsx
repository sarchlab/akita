import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import GanttChart from "../components/charts/GanttChart";
import TaskDetail from "../components/TaskDetail";
import { Button } from "../components/ui/button";
import { SidePanel } from "../components/ui/side-panel";
import { Input } from "../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { useComponentNames } from "../hooks/useComponentNames";
import { useSegments } from "../hooks/useSegments";
import { useSimulationRange } from "../hooks/useSimulationRange";
import { useTraceData } from "../hooks/useTraceData";
import type { Task } from "../types/task";
import { mergeParams } from "../utils/viewState.mjs";

export default function TaskChartPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  // URL is the source of truth for the filters; selectedTask is local because the
  // detail pane needs the full Task object (its id is mirrored to the `sel` param).
  const taskId = searchParams.get("id") ?? "";
  const component = searchParams.get("where") ?? "";
  const kind = searchParams.get("kind") ?? "";
  const sel = searchParams.get("sel") ?? "";
  const [taskInput, setTaskInput] = useState(taskId);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const { names } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segmentsData } = useSegments();

  const replaceParams = (patch: Record<string, string | undefined>) =>
    setSearchParams((prev) => mergeParams("/task", prev, patch), { replace: true });

  const mainQuery = useMemo(() => (taskId ? { id: taskId } : {}), [taskId]);
  const { tasks: mainTasks, loading: mainLoading } = useTraceData(mainQuery);
  const mainTask = mainTasks[0] ?? null;
  const parentQuery = useMemo(() => (mainTask?.parent_id ? { id: String(mainTask.parent_id) } : {}), [mainTask?.parent_id]);
  const { tasks: parentTasks } = useTraceData(parentQuery);
  const parentTask = parentTasks[0] ?? null;
  const childQuery = useMemo(() => (mainTask ? { parentId: String(mainTask.id) } : {}), [mainTask]);
  const { tasks: childTasks } = useTraceData(childQuery);

  const browseQuery = useMemo(
    () =>
      taskId
        ? {}
        : {
            where: component || undefined,
            kind: kind || undefined,
            startTime,
            endTime,
          },
    [component, endTime, kind, startTime, taskId],
  );
  const { tasks: browseTasks, loading: browseLoading } = useTraceData(browseQuery);

  const displayTasks = taskId ? childTasks : browseTasks;

  // Keep the task-id input box in sync when the active task changes (e.g. via
  // back/forward or an external link).
  useEffect(() => {
    setTaskInput(taskId);
  }, [taskId]);

  // Restore the selected task (the `sel` param) once its data has loaded — in both
  // browse mode (from the matching tasks) and detail mode (from main/parent/child).
  // With no `sel`, detail mode defaults to the main task.
  useEffect(() => {
    if (sel) {
      const pool = taskId ? [mainTask, parentTask, ...childTasks] : browseTasks;
      const found = pool.find((task) => task && String(task.id) === sel);
      if (found) {
        setSelectedTask((current) => (current && String(current.id) === sel ? current : found));
        return;
      }
    }
    if (taskId && mainTask) setSelectedTask(mainTask);
  }, [sel, taskId, mainTask?.id, parentTask?.id, childTasks, browseTasks]);

  const selectTask = useCallback(
    (task: Task) => {
      setSelectedTask(task);
      setSearchParams((prev) => mergeParams("/task", prev, { sel: String(task.id) }), { replace: true });
    },
    [setSearchParams],
  );

  const navigateToTask = useCallback(
    (id: string) => {
      // Entering task-detail mode: keep only the task id; drop browse-only filters
      // and selection so the URL carries no params that don't affect the view.
      setSearchParams((prev) => mergeParams("/task", prev, { id, where: undefined, kind: undefined, sel: undefined }));
      setSelectedTask(null);
    },
    [setSearchParams],
  );

  const selectedId = selectedTask?.id ?? (sel || null);

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <form
          className="flex min-h-12 flex-wrap items-center gap-2 border-b bg-white px-3 py-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (taskInput.trim()) navigateToTask(taskInput.trim());
          }}
        >
          <Input className="w-72" placeholder="Task ID" value={taskInput} onChange={(event) => setTaskInput(event.target.value)} />
          <Button type="submit">Go</Button>
          <Select
            value={component || "__all__"}
            onValueChange={(value) => {
              const next = value === "__all__" ? "" : value;
              // Switch to browse mode for the chosen component: drop the active
              // task id and selection, but keep the kind filter.
              replaceParams({ where: next || undefined, id: undefined, sel: undefined });
              setSelectedTask(null);
            }}
          >
            <SelectTrigger className="w-72">
              <SelectValue placeholder="Component" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">All Components</SelectItem>
              {names.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            className="w-44"
            placeholder="Kind filter"
            value={kind}
            onChange={(event) =>
              // The kind filter only applies to browse mode; typing it leaves
              // task-detail mode rather than adding a param with no effect.
              replaceParams({ kind: event.target.value || undefined, id: undefined, sel: undefined })
            }
          />
          {(mainLoading || browseLoading) && <span className="text-sm text-muted-foreground">Loading...</span>}
        </form>

        <div className="min-h-0 flex-1">
          <GanttChart
            tasks={displayTasks}
            mainTask={taskId ? mainTask : null}
            parentTask={taskId ? parentTask : null}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            selectedId={selectedId}
            onSelectTask={selectTask}
            onOpenTask={(task) => navigateToTask(String(task.id))}
          />
        </div>
      </div>

      <SidePanel className="w-96 overflow-auto p-4">
        <TaskDetail task={selectedTask} onNavigateToTask={navigateToTask} />
        {!taskId && browseTasks.length > 0 ? (
          <div className="mt-4 space-y-1">
            <div className="text-sm font-semibold">Matching Tasks</div>
            {browseTasks.slice(0, 60).map((task) => (
              <button
                key={task.id}
                type="button"
                className="block w-full rounded-md border bg-white p-2 text-left text-xs hover:bg-muted"
                onClick={() => selectTask(task)}
                onDoubleClick={() => navigateToTask(String(task.id))}
              >
                <div className="font-medium">{task.kind}</div>
                <div className="truncate text-muted-foreground">{task.what}</div>
              </button>
            ))}
          </div>
        ) : null}
      </SidePanel>
    </div>
  );
}
