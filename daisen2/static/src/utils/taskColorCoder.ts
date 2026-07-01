import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

// How tasks are grouped for coloring (and for the task-count chart's bands):
// by their kind alone, or by the finer "kind-what" pair. Must stay in sync with
// the server's component_timeline `group` param so a band's key matches the key
// a task computes here.
export type ColorMode = "kind" | "kind-what";

// Tasks and blocking-reason milestones are colored from two separate families so
// the two are tellable apart at a glance (they also share the kind / kind-what
// keying and could otherwise collide): tasks a cool cubehelix, milestones a warm
// one. Each family spans its own scale, so within a group colors stay distinct.
export type Palette = "task" | "milestone";

export function taskColorKey(
  task: Pick<Task, "kind" | "what">,
  mode: ColorMode = "kind-what",
): string {
  return mode === "kind" ? task.kind : `${task.kind}-${task.what}`;
}

function paletteScale(palette: Palette) {
  // Milestones: a warm amber→orange→red ramp. Tasks: a cool cubehelix (blue→purple).
  // The two families are easy to tell apart even when a key string collides.
  return palette === "milestone"
    ? chroma.scale(["#fcd34d", "#f59e0b", "#ea580c", "#c2410c", "#9f1239"]).mode("lab")
    : chroma.cubehelix().start(220).rotations(0.5).gamma(0.7).lightness([0.3, 0.74]).scale();
}

// buildColorMapFromKeys assigns each distinct key a color from the given palette's
// scale. Pass the keys of one family at a time (task "kind-what" keys, or
// blocking-reason keys) so each family gets the whole scale to itself.
export function buildColorMapFromKeys(keys: string[], palette: Palette = "task"): ColorMap {
  const uniqueKeys = Array.from(new Set(keys)).sort();
  const colors = paletteScale(palette).colors(uniqueKeys.length + 1);
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
