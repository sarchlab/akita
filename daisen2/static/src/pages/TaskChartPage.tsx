import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import GanttChart from "../components/charts/GanttChart";
import TaskDetail from "../components/TaskDetail";
import { SidePanel } from "../components/ui/side-panel";
import { useSegments } from "../hooks/useSegments";
import { useTraceData } from "../hooks/useTraceData";
import type { Task } from "../types/task";
import { mergeParams } from "../utils/viewState.mjs";

// The task view is a detail view, always reached with a task `id` (from the
// component view's inspect icon, "Open Parent", or opening a task from the
// chart). `id` selects the task whose parent, children and milestones are
// charted; `sel` is the task currently selected within that chart, shown in the
// detail panel. A bare /task (no id) renders the chart's empty state.
export default function TaskChartPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const taskId = searchParams.get("id") ?? "";
  const sel = searchParams.get("sel") ?? "";
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const { data: segmentsData } = useSegments();

  const mainQuery = useMemo(() => (taskId ? { id: taskId } : {}), [taskId]);
  const { tasks: mainTasks } = useTraceData(mainQuery);
  const mainTask = mainTasks[0] ?? null;
  const parentQuery = useMemo(() => (mainTask?.parent_id ? { id: String(mainTask.parent_id) } : {}), [mainTask?.parent_id]);
  const { tasks: parentTasks } = useTraceData(parentQuery);
  const parentTask = parentTasks[0] ?? null;
  const childQuery = useMemo(() => (mainTask ? { parentId: String(mainTask.id) } : {}), [mainTask]);
  const { tasks: childTasks } = useTraceData(childQuery);

  // Restore the selected task (the `sel` param) once its data has loaded; with no
  // `sel`, default the detail panel to the main task.
  useEffect(() => {
    if (sel) {
      const found = [mainTask, parentTask, ...childTasks].find((task) => task && String(task.id) === sel);
      if (found) {
        setSelectedTask((current) => (current && String(current.id) === sel ? current : found));
        return;
      }
    }
    if (taskId && mainTask) setSelectedTask(mainTask);
  }, [sel, taskId, mainTask?.id, parentTask?.id, childTasks]);

  const selectTask = useCallback(
    (task: Task) => {
      setSelectedTask(task);
      setSearchParams((prev) => mergeParams("/task", prev, { sel: String(task.id) }), { replace: true });
    },
    [setSearchParams],
  );

  // Open a different task by id (its parent, or a child), clearing the prior
  // in-chart selection.
  const navigateToTask = useCallback(
    (id: string) => {
      setSearchParams((prev) => mergeParams("/task", prev, { id, sel: undefined }));
      setSelectedTask(null);
    },
    [setSearchParams],
  );

  const selectedId = selectedTask?.id ?? (sel || null);

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="min-h-0 flex-1">
          <GanttChart
            tasks={childTasks}
            mainTask={mainTask}
            parentTask={parentTask}
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
      </SidePanel>
    </div>
  );
}
