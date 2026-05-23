import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

export function taskColorKey(task: Pick<Task, "kind" | "what">): string {
  return `${task.kind}-${task.what}`;
}

export function buildColorMap(tasks: Task[]): ColorMap {
  const keys = Array.from(new Set(tasks.map(taskColorKey))).sort();
  const colors = chroma.cubehelix().gamma(0.7).lightness([0.1, 0.7]).scale().colors(keys.length + 1);
  return keys.reduce<ColorMap>((map, key, index) => {
    map[key] = colors[index + 1] ?? "#999999";
    return map;
  }, {});
}

export function lookupColor(map: ColorMap, task: Task): string {
  return map[taskColorKey(task)] ?? "#999999";
}
