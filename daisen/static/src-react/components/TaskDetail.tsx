import type { Task } from "../types/task";
import { smartString } from "../utils/smartValue";

interface TaskDetailProps {
  task: Task | null;
  onNavigateToTask?: (taskId: string) => void;
}

export default function TaskDetail({ task, onNavigateToTask }: TaskDetailProps) {
  if (!task) {
    return (
      <div className="p-3 text-muted">
        <em>Click a task bar to see details.</em>
      </div>
    );
  }

  const duration = task.end_time - task.start_time;

  return (
    <div className="p-3" style={{ fontSize: 14 }}>
      <h5 className="mb-3">
        {task.kind} – {task.what}
      </h5>

      <dl className="row mb-2" style={{ fontSize: 13 }}>
        <dt className="col-4">ID</dt>
        <dd className="col-8 text-break" style={{ fontSize: 12 }}>
          {task.id}
        </dd>

        <dt className="col-4">Kind</dt>
        <dd className="col-8">{task.kind}</dd>

        <dt className="col-4">What</dt>
        <dd className="col-8">{task.what}</dd>

        <dt className="col-4">Location</dt>
        <dd className="col-8">{task.location}</dd>

        <dt className="col-4">Start</dt>
        <dd className="col-8">{smartString(task.start_time)}</dd>

        <dt className="col-4">End</dt>
        <dd className="col-8">{smartString(task.end_time)}</dd>

        <dt className="col-4">Duration</dt>
        <dd className="col-8">{smartString(duration)}</dd>
      </dl>

      {task.parent_id && (
        <div className="mb-3">
          <strong>Parent:</strong>{" "}
          <button
            className="btn btn-link btn-sm p-0"
            onClick={() => onNavigateToTask?.(task.parent_id)}
          >
            {task.parent_id}
          </button>
        </div>
      )}

      {task.steps && task.steps.length > 0 && (
        <>
          <h6>Milestones ({task.steps.length})</h6>
          <table className="table table-sm table-bordered" style={{ fontSize: 12 }}>
            <thead>
              <tr>
                <th>Time</th>
                <th>Kind</th>
                <th>What</th>
              </tr>
            </thead>
            <tbody>
              {task.steps.map((s, i) => (
                <tr key={i}>
                  <td>{smartString(s.time)}</td>
                  <td>{s.kind || "N/A"}</td>
                  <td>{s.what || "N/A"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </div>
  );
}
