import { Link } from "react-router-dom";
import type { Task } from "../types/task";
import { smartString } from "../utils/smartValue";
import { milestonesOf } from "../utils/milestoneViz";
import { breadcrumbSegments } from "../utils/locationTree";
import { encodeView } from "../utils/viewState.mjs";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";

interface TaskDetailProps {
  task: Task | null;
}

export default function TaskDetail({ task }: TaskDetailProps) {
  if (!task) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        Select a task to inspect its timing, location, and milestones.
      </div>
    );
  }

  // Each location token links to the component view at that level (the token's
  // dotted prefix), focused on this task's time window and with the task
  // pre-selected. Higher-level tokens open the aggregated parent location.
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

  const milestones = milestonesOf(task.steps);

  return (
    <Card className="m-3 rounded-md shadow-none">
      <CardHeader className="pb-2">
        <CardTitle className="break-all text-sm">{task.kind}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <dl className="grid grid-cols-[6rem_1fr] gap-x-3 gap-y-2">
          <dt className="text-muted-foreground">ID</dt>
          <dd className="break-all">{task.id}</dd>
          <dt className="text-muted-foreground">What</dt>
          <dd className="break-all">{task.what || "-"}</dd>
          <dt className="text-muted-foreground">Location</dt>
          <dd className="min-w-0 break-all">
            {task.location ? (
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
            )}
          </dd>
          <dt className="text-muted-foreground">Start</dt>
          <dd>{smartString(task.start_time)}</dd>
          <dt className="text-muted-foreground">End</dt>
          <dd>{smartString(task.end_time)}</dd>
          <dt className="text-muted-foreground">Duration</dt>
          <dd>{smartString(task.end_time - task.start_time)}</dd>
        </dl>

        {milestones.length ? (
          <div>
            <div className="mb-2 font-medium">Milestones</div>
            <div className="space-y-2">
              {milestones.map((step, index) => (
                <div key={`${step.time}-${index}`} className="rounded-md border bg-muted/40 p-2">
                  <div className="font-medium">{step.kind}</div>
                  <div className="text-muted-foreground">{step.what}</div>
                  <div className="text-xs text-muted-foreground">{smartString(step.time)}</div>
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
