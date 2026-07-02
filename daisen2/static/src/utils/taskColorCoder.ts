import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

// How tasks are grouped for coloring (and for the task-count chart's bands):
// by their kind alone, or by the finer "kind-what" pair. Must stay in sync with
// the server's component_timeline `group` param so a band's key matches the key
// a task computes here.
export type ColorMode = "kind" | "kind-what";

// Tasks and blocking-reason milestones are colored from two full cubehelix turns.
// The families start at different phases so task and milestone colors do not line
// up exactly, but each legend gets the full helix instead of a narrow hue ramp.
export type Palette = "task" | "milestone";

export function taskColorKey(
  task: Pick<Task, "kind" | "what">,
  mode: ColorMode = "kind-what",
): string {
  return mode === "kind" ? task.kind : `${task.kind}-${task.what}`;
}

function paletteScale(palette: Palette) {
  return chroma
    .cubehelix()
    .start(palette === "milestone" ? 30 : 210)
    .rotations(1)
    .gamma(0.85)
    .lightness([0.32, 0.76])
    .scale();
}

// buildColorMapFromKeys assigns each distinct key a color from the given palette's
// scale. Pass the keys of one family at a time (task "kind-what" keys, or
// blocking-reason keys) so each family gets the whole scale to itself.
export function buildColorMapFromKeys(keys: string[], palette: Palette = "task"): ColorMap {
  const uniqueKeys = Array.from(new Set(keys)).sort();
  const scale = paletteScale(palette);
  return uniqueKeys.reduce<ColorMap>((map, key, index) => {
    map[key] = scale((index + 0.5) / uniqueKeys.length).hex();
    return map;
  }, {});
}

export function lookupColor(
  map: ColorMap,
  task: Task,
  mode: ColorMode = "kind-what",
): string {
  return map[taskColorKey(task, mode)] ?? "#999999";
}
