import { useState, useCallback, useMemo, useEffect } from "react";
import { useSearchParams } from "react-router";
import GanttChart from "../components/charts/GanttChart";
import TaskDetail from "../components/TaskDetail";
import { useTraceData } from "../hooks/useTraceData";
import { useSegments } from "../hooks/useSegments";
import { useComponentNames } from "../hooks/useComponentNames";
import type { Task } from "../types/task";

export default function TaskChartPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const taskId = searchParams.get("id") ?? "";

  /* ---------- Selector state ---------- */
  const [selectedKind, setSelectedKind] = useState("");
  const [selectedWhere, setSelectedWhere] = useState("");

  /* ---------- Component names ---------- */
  const { names: compNames } = useComponentNames();

  /* ---------- Fetch main task by id ---------- */
  const idQuery = useMemo(() => (taskId ? { id: taskId } : {}), [taskId]);
  const { tasks: mainTasks, loading: mainLoading } = useTraceData(idQuery);
  const mainTask = mainTasks.length > 0 ? mainTasks[0] : null;

  /* ---------- Fetch parent task ---------- */
  const parentQuery = useMemo(
    () => (mainTask?.parent_id ? { id: mainTask.parent_id } : {}),
    [mainTask?.parent_id],
  );
  const { tasks: parentTasks } = useTraceData(parentQuery);
  const parentTask = parentTasks.length > 0 ? parentTasks[0] : null;

  /* ---------- Fetch sub tasks ---------- */
  const subQuery = useMemo(
    () => (mainTask?.id ? { parentId: mainTask.id } : {}),
    [mainTask?.id],
  );
  const { tasks: subTasks } = useTraceData(subQuery);

  /* ---------- Fetch by kind/where (browse mode) ---------- */
  const browseQuery = useMemo(() => {
    if (taskId) return {};
    if (selectedKind && selectedWhere)
      return { kind: selectedKind, where: selectedWhere };
    if (selectedWhere) return { where: selectedWhere };
    return {};
  }, [taskId, selectedKind, selectedWhere]);
  const { tasks: browseTasks, loading: browseLoading } =
    useTraceData(browseQuery);

  /* ---------- Determine display tasks ---------- */
  const displayMain = mainTask ?? (browseTasks.length > 0 ? browseTasks[0] : null);

  /* ---------- Segments ---------- */
  const { data: segData } = useSegments();

  /* ---------- Selected task for detail panel ---------- */
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);

  // Auto-select the main task when it loads
  useEffect(() => {
    if (displayMain && !selectedTask) {
      setSelectedTask(displayMain);
    }
  }, [displayMain, selectedTask]);

  /* ---------- Handlers ---------- */
  const handleSelectTask = useCallback(
    (task: Task) => {
      setSelectedTask(task);
      // If clicking a subtask, navigate to it
    },
    [],
  );

  const handleNavigateToTask = useCallback(
    (id: string) => {
      setSearchParams({ id });
      setSelectedTask(null);
    },
    [setSearchParams],
  );

  const handleSearchSubmit = useCallback(
    (e: React.FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      const form = new FormData(e.currentTarget);
      const id = (form.get("taskIdInput") as string) ?? "";
      if (id.trim()) {
        setSearchParams({ id: id.trim() });
        setSelectedTask(null);
      }
    },
    [setSearchParams],
  );

  const handleDoubleClickTask = useCallback(
    (task: Task) => {
      setSearchParams({ id: task.id });
      setSelectedTask(null);
    },
    [setSearchParams],
  );

  /* ---------- Loading states ---------- */
  const loading = mainLoading || browseLoading;
  const hasData =
    displayMain !== null || browseTasks.length > 0;

  return (
    <div className="d-flex" style={{ height: "calc(100vh - 76px)" }}>
      {/* ── Left panel: chart ─────────────────────────────────── */}
      <div
        className="flex-grow-1 d-flex flex-column"
        style={{ overflow: "hidden" }}
      >
        {/* Search bar */}
        <div className="border-bottom p-2 d-flex gap-2 align-items-center flex-wrap">
          <form
            className="d-flex gap-1 align-items-center"
            onSubmit={handleSearchSubmit}
          >
            <input
              name="taskIdInput"
              className="form-control form-control-sm"
              placeholder="Task ID"
              defaultValue={taskId}
              style={{ width: 260 }}
            />
            <button className="btn btn-sm btn-primary" type="submit">
              Go
            </button>
          </form>

          <select
            className="form-select form-select-sm"
            style={{ width: 180 }}
            value={selectedWhere}
            onChange={(e) => {
              setSelectedWhere(e.target.value);
              if (!taskId) setSelectedTask(null);
            }}
          >
            <option value="">— Component —</option>
            {compNames.map((n) => (
              <option key={n} value={n}>
                {n}
              </option>
            ))}
          </select>

          <input
            className="form-control form-control-sm"
            placeholder="Kind filter"
            style={{ width: 140 }}
            value={selectedKind}
            onChange={(e) => {
              setSelectedKind(e.target.value);
              if (!taskId) setSelectedTask(null);
            }}
          />

          {loading && (
            <span className="spinner-border spinner-border-sm text-primary" />
          )}
        </div>

        {/* Chart area */}
        <div className="flex-grow-1" style={{ position: "relative" }}>
          {hasData ? (
            <GanttChart
              mainTask={displayMain}
              parentTask={taskId ? parentTask : null}
              subTasks={taskId ? subTasks : browseTasks.slice(1)}
              segments={segData?.segments ?? []}
              segmentsEnabled={segData?.enabled ?? false}
              onSelectTask={handleSelectTask}
              onHoverTask={() => {}}
            />
          ) : (
            <div className="d-flex align-items-center justify-content-center h-100 text-muted">
              {loading ? (
                <div className="text-center">
                  <div className="spinner-border mb-2" />
                  <div>Loading trace data…</div>
                </div>
              ) : (
                <div className="text-center">
                  <p className="mb-1">No tasks to display.</p>
                  <p className="small">
                    Enter a Task ID above, or select a component to browse
                    tasks.
                  </p>
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* ── Right panel: detail ──────────────────────────────── */}
      <div
        className="border-start bg-light"
        style={{ width: 320, minWidth: 260, overflowY: "auto" }}
      >
        <TaskDetail
          task={selectedTask}
          onNavigateToTask={handleNavigateToTask}
        />

        {/* browse task list when in browse mode */}
        {!taskId && browseTasks.length > 1 && (
          <div className="p-3 border-top">
            <h6>
              Matching tasks ({browseTasks.length})
            </h6>
            <div
              className="list-group list-group-flush"
              style={{ fontSize: 12, maxHeight: 300, overflowY: "auto" }}
            >
              {browseTasks.slice(0, 50).map((t) => (
                <button
                  key={t.id}
                  type="button"
                  className={`list-group-item list-group-item-action ${
                    selectedTask?.id === t.id ? "active" : ""
                  }`}
                  onClick={() => handleDoubleClickTask(t)}
                >
                  <div className="fw-semibold">{t.kind}</div>
                  <div className="text-truncate">{t.what}</div>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
