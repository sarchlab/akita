import type { Task } from "../types/task";

/**
 * Assigns non-overlapping Y indices to tasks so they can be stacked
 * vertically in a Gantt chart without overlap.
 *
 * Returns the maximum Y index used.
 */
export function assignYIndices(tasks: Task[]): number {
  const assignment: Task[][] = [];
  let maxYIndex = 0;

  const sorted = [...tasks].sort((a, b) => a.start_time - b.start_time);

  for (const t of sorted) {
    let index = 0;
    while (hasConflict(t, assignment[index])) {
      index++;
    }

    if (assignment.length <= index) {
      assignment.push([]);
    }
    assignment[index].push(t);
    t.yIndex = index;

    if (index > maxYIndex) {
      maxYIndex = index;
    }
  }

  return maxYIndex;
}

function hasConflict(task: Task, row: Task[] | undefined): boolean {
  if (!row) return false;

  for (const t of row) {
    if (
      (t.start_time <= task.start_time && t.end_time > task.start_time) ||
      (t.start_time < task.end_time && t.end_time >= task.end_time) ||
      (task.start_time <= t.start_time && task.end_time >= t.end_time) ||
      (task.start_time >= t.start_time && task.end_time <= t.end_time)
    ) {
      return true;
    }
  }
  return false;
}
