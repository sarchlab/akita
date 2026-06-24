import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

// How tasks are grouped for coloring (and for the task-count chart's bands):
// by their kind alone, or by the finer "kind-what" pair. Must stay in sync with
// the server's component_timeline `group` param so a band's key matches the key
// a task computes here.
export type ColorMode = "kind" | "kind-what";

export function taskColorKey(
  task: Pick<Task, "kind" | "what">,
  mode: ColorMode = "kind-what",
): string {
  return mode === "kind" ? task.kind : `${task.kind}-${task.what}`;
}

// buildColorMapFromKeys assigns each distinct key a color from a single
// cubehelix scale. Pass the union of every key that needs a color (task
// "kind-what" keys plus blocking-reason kinds) so they are all globally distinct
// and drawn from the same mechanism.
export function buildColorMapFromKeys(keys: string[]): ColorMap {
  const uniqueKeys = Array.from(new Set(keys)).sort();
  const colors = chroma.cubehelix().gamma(0.7).lightness([0.1, 0.7]).scale().colors(uniqueKeys.length + 1);
  return uniqueKeys.reduce<ColorMap>((map, key, index) => {
    map[key] = colors[index + 1] ?? "#999999";
    return map;
  }, {});
}

export function buildColorMap(tasks: Task[]): ColorMap {
  return buildColorMapFromKeys(tasks.map((task) => taskColorKey(task)));
}

export function lookupColor(
  map: ColorMap,
  task: Task,
  mode: ColorMode = "kind-what",
): string {
  return map[taskColorKey(task, mode)] ?? "#999999";
}
