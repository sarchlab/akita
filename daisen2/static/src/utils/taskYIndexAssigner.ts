import type { Task } from "../types/task";

export function assignYIndices(tasks: Task[]): number {
  const lanes: number[] = [];
  const sorted = [...tasks].sort((a, b) => a.start_time - b.start_time || a.end_time - b.end_time);

  for (const task of sorted) {
    let lane = lanes.findIndex((endTime) => endTime <= task.start_time);
    if (lane < 0) {
      lane = lanes.length;
      lanes.push(task.end_time);
    } else {
      lanes[lane] = task.end_time;
    }
    task.yIndex = lane;
  }

  return Math.max(0, lanes.length - 1);
}
