import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { ExternalLink } from "lucide-react";
import type { Task } from "../types/task";
import type { SelectedMilestone } from "../utils/milestoneViz";
import { resourceHrefForWhat, type TimeRange } from "../utils/resourceLinks";
import { smartString } from "../utils/smartValue";
import TaskDetail from "./TaskDetail";
import { SectionLabel } from "./Legend";

// The right-panel detail for the current selection, shared by the component and
// task views so both read identically. A selected blocking milestone takes over
// from the task and shows the same boxed "label + key/value rows" card.
export default function SelectedTaskSection({
  task,
  milestone,
  resourceRange,
}: {
  task: Task | null;
  milestone: SelectedMilestone | null;
  resourceRange?: TimeRange;
}) {
  if (milestone) {
    const whatHref =
      milestone.kind === "hardware_resource" && milestone.what
        ? resourceHrefForWhat(milestone.what, resourceRange)
        : null;
    const whatValue = whatHref ? (
      <Link
        to={whatHref}
        className="group inline-flex min-w-0 items-center gap-1 text-primary underline-offset-4 hover:underline"
        title={`Open the resource view for ${milestone.what}`}
      >
        <span className="break-all">{milestone.what}</span>
        <ExternalLink className="h-3 w-3 shrink-0 text-muted-foreground/60 transition-colors group-hover:text-primary" />
      </Link>
    ) : (
      milestone.what || "-"
    );
    const rows: { label: string; value: ReactNode; numeric?: boolean }[] = [
      { label: "Reason", value: milestone.kind },
      { label: "What", value: whatValue },
      { label: "Released", value: smartString(milestone.time), numeric: true },
      { label: "Blocked for", value: smartString(milestone.blockedFor), numeric: true },
    ];
    return (
      <section>
        <SectionLabel>Selected milestone</SectionLabel>
        <div className="mt-2 rounded-lg border bg-muted/30 p-3">
          <div className="mb-2 break-all text-sm font-semibold">blocked on {milestone.kind}</div>
          <dl className="space-y-1.5 text-xs">
            {rows.map(({ label, value, numeric }) => (
              <div key={label} className="grid grid-cols-[5.5rem_1fr] gap-x-3">
                <dt className="text-muted-foreground">{label}</dt>
                <dd className={`break-all font-medium ${numeric ? "tabular-nums" : ""}`}>{value}</dd>
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
