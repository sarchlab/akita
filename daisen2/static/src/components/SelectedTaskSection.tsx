import type { Task } from "../types/task";
import type { SelectedMilestone } from "../utils/milestoneViz";
import { smartString } from "../utils/smartValue";
import TaskDetail from "./TaskDetail";
import { SectionLabel } from "./Legend";

// The right-panel detail for the current selection, shared by the component and
// task views so both read identically. A selected blocking milestone takes over
// from the task and shows the same boxed "label + key/value rows" card.
export default function SelectedTaskSection({
  task,
  milestone,
}: {
  task: Task | null;
  milestone: SelectedMilestone | null;
}) {
  if (milestone) {
    const rows: [string, string][] = [
      ["Reason", milestone.kind],
      ["What", milestone.what || "-"],
      ["Released", smartString(milestone.time)],
      ["Blocked for", smartString(milestone.blockedFor)],
    ];
    return (
      <section>
        <SectionLabel>Selected milestone</SectionLabel>
        <div className="mt-2 rounded-lg border bg-muted/30 p-3">
          <div className="mb-2 break-all text-sm font-semibold">blocked on {milestone.kind}</div>
          <dl className="space-y-1.5 text-xs">
            {rows.map(([label, value]) => (
              <div key={label} className="grid grid-cols-[5.5rem_1fr] gap-x-3">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className="break-all font-medium tabular-nums">{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      </section>
    );
  }

  // Nothing selected — just the inline prompt, no section heading.
  if (!task) return <TaskDetail task={null} />;
  // The selected task uses the shared TaskDetail under a heading that mirrors the
  // "Selected milestone" panel.
  return (
    <section>
      <SectionLabel>Selected task</SectionLabel>
      <TaskDetail task={task} />
    </section>
  );
}
