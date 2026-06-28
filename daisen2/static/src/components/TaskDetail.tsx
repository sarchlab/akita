import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Search } from "lucide-react";
import type { Task } from "../types/task";
import { smartString } from "../utils/smartValue";
import { milestonesOf } from "../utils/milestoneViz";
import { breadcrumbSegments } from "../utils/locationTree";
import { encodeView } from "../utils/viewState.mjs";

interface TaskDetailProps {
  task: Task | null;
}

// The selected-task panel. Shares the boxed "label + key/value rows" styling with
// the selected-milestone panel (see SelectedTaskSection) so the two read as one
// family. Each location token links to the component view at that level.
export default function TaskDetail({ task }: TaskDetailProps) {
  if (!task) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        Select a task to inspect its timing, location, and milestones.
      </div>
    );
  }

  const duration = task.end_time - task.start_time;
  const padding = duration > 0 ? duration * 0.2 : 0;
  const componentHref = (location: string) =>
    encodeView({
      route: "component",
      name: location,
      taskId: String(task.id),
      ...(duration > 0
        ? { startTime: task.start_time - padding, endTime: task.end_time + padding }
        : {}),
    });

  // Sorted by time so each milestone's "blocked for" interval — from the previous
  // milestone (or the task start) to this one — is well defined.
  const milestones = milestonesOf(task.steps).sort((a, b) => a.time - b.time);

  const locationValue: ReactNode = task.location ? (
    <span className="inline-flex flex-wrap items-center">
      {breadcrumbSegments(task.location).map((seg, index) => (
        <span key={seg.path} className="inline-flex items-center">
          {index > 0 ? <span className="mx-0.5 text-muted-foreground">.</span> : null}
          <Link
            to={componentHref(seg.path)}
            className="rounded px-0.5 text-primary hover:underline"
            title={`Open ${seg.path} in the component view`}
          >
            {seg.label}
          </Link>
        </span>
      ))}
    </span>
  ) : (
    "-"
  );

  // [label, value, numeric] — numeric values get tabular-nums for alignment.
  const rows: [string, ReactNode, boolean][] = [
    ["ID", task.id, true],
    ["What", task.what || "-", false],
    ["Location", locationValue, false],
    ["Start", smartString(task.start_time), true],
    ["End", smartString(task.end_time), true],
    ["Duration", smartString(duration), true],
  ];

  return (
    <div className="mt-2 rounded-lg border bg-muted/30 p-3">
      <div className="mb-2 flex items-start justify-between gap-2">
        <span className="break-all text-sm font-semibold">{task.kind}</span>
        <Link
          to={encodeView({ route: "task", id: String(task.id) })}
          className="shrink-0 rounded border bg-background p-1 text-muted-foreground hover:text-primary"
          title="Open this task in the task view"
          aria-label="Open this task in the task view"
        >
          <Search className="h-3.5 w-3.5" />
        </Link>
      </div>
      <dl className="space-y-1.5 text-xs">
        {rows.map(([label, value, numeric]) => (
          <div key={label} className="grid grid-cols-[5.5rem_1fr] gap-x-3">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className={`min-w-0 break-all font-medium${numeric ? " tabular-nums" : ""}`}>{value}</dd>
          </div>
        ))}
      </dl>

      {milestones.length ? (
        <div className="mt-3">
          <div className="mb-1.5 text-xs font-medium">Milestones</div>
          <div className="space-y-1.5">
            {milestones.map((step, index) => {
              const intervalStart = index === 0 ? task.start_time : milestones[index - 1].time;
              const blockedFor = step.time - intervalStart;
              return (
                <div key={`${step.time}-${index}`} className="rounded-md border bg-background/60 p-2 text-xs">
                  <div className="break-all font-medium">{step.kind}</div>
                  <div className="break-all text-muted-foreground">{step.what}</div>
                  <div className="text-muted-foreground">blocked for {smartString(blockedFor)}</div>
                </div>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}
