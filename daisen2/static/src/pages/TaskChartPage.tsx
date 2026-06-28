import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import GanttChart from "../components/charts/GanttChart";
import Legend from "../components/Legend";
import TaskDetail from "../components/TaskDetail";
import { SidePanel } from "../components/ui/side-panel";
import { useSegments } from "../hooks/useSegments";
import { useTaskHierarchy } from "../hooks/useTaskHierarchy";
import type { Task } from "../types/task";
import { buildColorMapFromKeys, taskColorKey } from "../utils/taskColorCoder";
import type { ColorMode } from "../utils/taskColorCoder";
import { milestonesOf } from "../utils/milestoneViz";
import { mergeParams } from "../utils/viewState.mjs";

// The task view is a detail view, always reached with a task `id`. It charts the
// task with its full ancestor chain above it and its descendant subtree below
// (loaded level by level). `sel` is the task currently selected within the chart,
// shown in the detail panel. A bare /task (no id) renders the empty state.
export default function TaskChartPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const taskId = searchParams.get("id") ?? "";
  const sel = searchParams.get("sel") ?? "";
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [colorMode, setColorMode] = useState<ColorMode>("kind-what");
  // Gates the default-to-main selection to once per task id, so an explicit
  // deselect (clicking the chart background) is not immediately undone.
  const autoSelectedFor = useRef<string | null>(null);
  const { data: segmentsData } = useSegments();
  const { mainTask, ancestors, levels, atLeaves, loading, expanding, expandNext } = useTaskHierarchy(taskId);

  const allTasks = useMemo(
    () => [...ancestors, ...(mainTask ? [mainTask] : []), ...levels.flat()],
    [ancestors, mainTask, levels],
  );

  // Task color keys and blocking reasons present in the chart, and a color map
  // over both — shared with GanttChart (so bars match) and the Legend (so swatches
  // match).
  const taskKeys = useMemo(() => {
    const keys: string[] = [];
    const seen = new Set<string>();
    for (const task of allTasks) {
      const key = taskColorKey(task, colorMode);
      if (!seen.has(key)) {
        seen.add(key);
        keys.push(key);
      }
    }
    return keys;
  }, [allTasks, colorMode]);
  const blockingReasons = useMemo(
    () => Array.from(new Set(milestonesOf(mainTask?.steps).map((step) => step.kind))),
    [mainTask],
  );
  const colorMap = useMemo(
    () => buildColorMapFromKeys([...taskKeys, ...blockingReasons]),
    [taskKeys, blockingReasons],
  );

  // Restore the selected task (the `sel` param) once its data has loaded; with no
  // `sel`, default the detail panel to the main task (once per task id).
  useEffect(() => {
    if (sel) {
      const found = allTasks.find((task) => String(task.id) === sel);
      if (found) {
        setSelectedTask((current) => (current && String(current.id) === sel ? current : found));
        return;
      }
    }
    if (taskId && mainTask && autoSelectedFor.current !== taskId) {
      autoSelectedFor.current = taskId;
      setSelectedTask(mainTask);
    }
  }, [sel, taskId, mainTask, allTasks]);

  const selectTask = useCallback(
    (task: Task) => {
      setSelectedTask(task);
      setSearchParams((prev) => mergeParams("/task", prev, { sel: String(task.id) }), { replace: true });
    },
    [setSearchParams],
  );

  // Open a different task by id (an ancestor or a descendant), recentering the
  // view on it and clearing the prior in-chart selection.
  const navigateToTask = useCallback(
    (id: string) => {
      setSearchParams((prev) => mergeParams("/task", prev, { id, sel: undefined }));
      setSelectedTask(null);
    },
    [setSearchParams],
  );

  // Clear the selection when the chart background is clicked.
  const deselect = useCallback(() => {
    autoSelectedFor.current = taskId;
    setSelectedTask(null);
    setSearchParams((prev) => mergeParams("/task", prev, { sel: undefined }), { replace: true });
  }, [setSearchParams, taskId]);

  const selectedId = selectedTask?.id ?? (sel || null);
  const canExpand = !loading && !atLeaves && levels.length > 0;

  return (
    <div className="flex h-full overflow-hidden">
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="min-h-0 flex-1">
          <GanttChart
            ancestors={ancestors}
            mainTask={mainTask}
            levels={levels}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            colorMap={colorMap}
            colorMode={colorMode}
            selectedId={selectedId}
            onSelectTask={selectTask}
            onOpenTask={(task) => navigateToTask(String(task.id))}
            onDeselect={deselect}
            canExpand={canExpand}
            expanding={expanding}
            onExpandNext={expandNext}
          />
        </div>
      </div>

      <SidePanel className="w-96 overflow-auto p-4">
        <TaskDetail task={selectedTask} />
        <div className="mt-2 border-t px-3 pt-3">
          <Legend
            taskKeys={taskKeys}
            colorMap={colorMap}
            blockingReasons={blockingReasons}
            colorMode={colorMode}
            onColorMode={setColorMode}
          />
        </div>
      </SidePanel>
    </div>
  );
}
