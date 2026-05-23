import type { Task } from "../types/task";
import { smartString } from "../utils/smartValue";
import { Button } from "./ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "./ui/card";

interface TaskDetailProps {
  task: Task | null;
  onNavigateToTask?: (id: string) => void;
}

export default function TaskDetail({ task, onNavigateToTask }: TaskDetailProps) {
  if (!task) {
    return (
      <div className="p-4 text-sm text-muted-foreground">
        Select a task to inspect its timing, location, and milestones.
      </div>
    );
  }

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
          <dd className="break-all">{task.location || "-"}</dd>
          <dt className="text-muted-foreground">Start</dt>
          <dd>{smartString(task.start_time)}</dd>
          <dt className="text-muted-foreground">End</dt>
          <dd>{smartString(task.end_time)}</dd>
          <dt className="text-muted-foreground">Duration</dt>
          <dd>{smartString(task.end_time - task.start_time)}</dd>
          <dt className="text-muted-foreground">Parent</dt>
          <dd className="break-all">{task.parent_id || "-"}</dd>
        </dl>

        {task.parent_id ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onNavigateToTask?.(String(task.parent_id))}
          >
            Open Parent
          </Button>
        ) : null}

        {task.steps?.length ? (
          <div>
            <div className="mb-2 font-medium">Milestones</div>
            <div className="space-y-2">
              {task.steps.map((step, index) => (
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
