import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import GanttChart from "../components/charts/GanttChart";
import TaskDetail from "../components/TaskDetail";
import { Button } from "../components/ui/button";
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

export default function TaskChartPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const taskId = searchParams.get("id") ?? "";
  const urlComponent = searchParams.get("where") ?? "";
  const [taskInput, setTaskInput] = useState(taskId);
  const [component, setComponent] = useState(urlComponent);
  const [kind, setKind] = useState("");
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const { names } = useComponentNames();
  const { startTime, endTime } = useSimulationRange();
  const { data: segmentsData } = useSegments();

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

  useEffect(() => {
    if (mainTask) setSelectedTask(mainTask);
  }, [mainTask?.id]);

  const navigateToTask = useCallback(
    (id: string) => {
      setSearchParams({ id });
      setTaskInput(id);
      setSelectedTask(null);
    },
    [setSearchParams],
  );

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
              setComponent(next);
              setSearchParams(next ? { where: next } : {});
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
          <Input className="w-44" placeholder="Kind filter" value={kind} onChange={(event) => setKind(event.target.value)} />
          {(mainLoading || browseLoading) && <span className="text-sm text-muted-foreground">Loading...</span>}
        </form>

        <div className="min-h-0 flex-1">
          <GanttChart
            tasks={displayTasks}
            mainTask={taskId ? mainTask : null}
            parentTask={taskId ? parentTask : null}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            onSelectTask={setSelectedTask}
            onOpenTask={(task) => navigateToTask(String(task.id))}
          />
        </div>
      </div>

      <aside className="daisen-side-column w-96 shrink-0">
        <TaskDetail task={selectedTask} onNavigateToTask={navigateToTask} />
        {!taskId && browseTasks.length > 0 ? (
          <div className="mt-4 space-y-1">
            <div className="text-sm font-semibold">Matching Tasks</div>
            {browseTasks.slice(0, 60).map((task) => (
              <button
                key={task.id}
                type="button"
                className="block w-full rounded-md border bg-white p-2 text-left text-xs hover:bg-muted"
                onClick={() => setSelectedTask(task)}
                onDoubleClick={() => navigateToTask(String(task.id))}
              >
                <div className="font-medium">{task.kind}</div>
                <div className="truncate text-muted-foreground">{task.what}</div>
              </button>
            ))}
          </div>
        ) : null}
      </aside>
    </div>
  );
}
