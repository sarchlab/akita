import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import GanttChart from "../components/charts/GanttChart";
import { TaskGanttHelp } from "../components/HelpTopics";
import Legend from "../components/Legend";
import SelectedTaskSection from "../components/SelectedTaskSection";
import TraceChartLayout from "../components/TraceChartLayout";
import { useSegments } from "../hooks/useSegments";
import { useTaskHierarchy } from "../hooks/useTaskHierarchy";
import type { Task } from "../types/task";
import { buildColorMapFromKeys, taskColorKey } from "../utils/taskColorCoder";
import type { ColorMode } from "../utils/taskColorCoder";
import { milestonesOf, type SelectedMilestone } from "../utils/milestoneViz";
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
  // A selected blocking milestone takes over the detail panel from the task
  // (mirrors the component view).
  const [selectedMilestone, setSelectedMilestone] = useState<SelectedMilestone | null>(null);
  const [colorMode, setColorMode] = useState<ColorMode>("kind-what");
  // Legend hover highlights, shared with the chart (same as the component view).
  const [highlightedKey, setHighlightedKey] = useState<string | null>(null);
  const [highlightedReason, setHighlightedReason] = useState<string | null>(null);
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
  // Every rendered row (ancestors, the main task, and all descendant levels) draws
  // its milestones, so gather reasons across all of them — otherwise a reason that
  // only a parent or child has would fall back to gray and be missing from the legend.
  const blockingReasons = useMemo(() => {
    const reasons = new Set<string>();
    for (const task of allTasks) {
      for (const step of milestonesOf(task.steps)) reasons.add(step.kind);
    }
    return Array.from(reasons);
  }, [allTasks]);
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
      setSelectedMilestone(null);
      setSearchParams((prev) => mergeParams("/task", prev, { sel: String(task.id) }), { replace: true });
    },
    [setSearchParams],
  );

  // Selecting a blocking milestone clears the task selection so the panel shows
  // the reason instead, and (via reasonHighlight) keeps that reason lit.
  const selectMilestone = useCallback(
    (milestone: SelectedMilestone | null) => {
      setSelectedMilestone(milestone);
      setSelectedTask(null);
      autoSelectedFor.current = taskId;
      setSearchParams((prev) => mergeParams("/task", prev, { sel: undefined }), { replace: true });
    },
    [setSearchParams, taskId],
  );

  // Open a different task by id (an ancestor or a descendant), recentering the
  // view on it and clearing the prior in-chart selection.
  const navigateToTask = useCallback(
    (id: string) => {
      setSearchParams((prev) => mergeParams("/task", prev, { id, sel: undefined }));
      setSelectedTask(null);
      setSelectedMilestone(null);
    },
    [setSearchParams],
  );

  // Clear the selection when the chart background is clicked.
  const deselect = useCallback(() => {
    autoSelectedFor.current = taskId;
    setSelectedTask(null);
    setSelectedMilestone(null);
    setSearchParams((prev) => mergeParams("/task", prev, { sel: undefined }), { replace: true });
  }, [setSearchParams, taskId]);

  const selectedId = selectedTask?.id ?? (sel || null);
  // A hovered reason wins; otherwise the selected milestone's reason stays lit.
  const reasonHighlight = highlightedReason ?? selectedMilestone?.kind ?? null;
  const canExpand = !loading && !atLeaves && levels.length > 0;

  return (
    <TraceChartLayout
      panel={
        <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-auto p-4">
          <SelectedTaskSection task={selectedTask} milestone={selectedMilestone} />
          <div className="-mx-4 border-t" />
          <Legend
            taskKeys={taskKeys}
            colorMap={colorMap}
            blockingReasons={blockingReasons}
            colorMode={colorMode}
            onColorMode={setColorMode}
            highlightedKey={highlightedKey}
            onHighlight={setHighlightedKey}
            highlightedReason={reasonHighlight}
            onHighlightReason={setHighlightedReason}
          />
        </div>
      }
    >
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <div className="relative min-h-0 flex-1">
          <GanttChart
            ancestors={ancestors}
            mainTask={mainTask}
            levels={levels}
            segments={segmentsData?.segments ?? []}
            segmentsEnabled={segmentsData?.enabled ?? false}
            colorMap={colorMap}
            colorMode={colorMode}
            selectedId={selectedMilestone ? null : selectedId}
            selectedMilestone={selectedMilestone}
            highlightedKey={highlightedKey}
            highlightedReason={reasonHighlight}
            onSelectTask={selectTask}
            onOpenTask={(task) => navigateToTask(String(task.id))}
            onSelectMilestone={selectMilestone}
            onDeselect={deselect}
            canExpand={canExpand}
            expanding={expanding}
            onExpandNext={expandNext}
          />
          <div className="absolute bottom-2 right-2 z-20" onPointerDown={(e) => e.stopPropagation()}>
            <TaskGanttHelp className="bg-white/85 p-1 shadow-sm ring-1 ring-slate-200 backdrop-blur-sm hover:bg-white" />
          </div>
        </div>
      </div>
    </TraceChartLayout>
  );
}
