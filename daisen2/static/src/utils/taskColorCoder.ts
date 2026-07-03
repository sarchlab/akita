import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

// How tasks are grouped for coloring (and for the task-count chart's bands):
// by their kind alone, or by the finer "kind-what" pair. Must stay in sync with
// the server's component_timeline `group` param so a band's key matches the key
// a task computes here.
export type ColorMode = "kind" | "kind-what";

// Tasks and blocking-reason milestones are colored from two cubehelix ramps
// starting at different phases, so the two legends read as different families
// while sharing the same muted overall tone.
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

// Keys are either a bare kind or the server's `Kind || '-' || What`, so the
// text before the first "-" is the kind either way.
function kindOfKey(key: string): string {
  const dash = key.indexOf("-");
  return dash === -1 ? key : key.slice(0, dash);
}

// shade spreads a kind's "what" variants around the base color by lightness,
// keeping the hue so the variants still read as one family. The span is kept
// modest so shades of pale helix colors do not wash out to white.
function shade(base: string, index: number, count: number): string {
  if (count <= 1) return base;
  const span = Math.min(1.4, 0.5 * (count - 1));
  const delta = -span / 2 + (index / (count - 1)) * span;
  return delta >= 0 ? chroma(base).brighten(delta).hex() : chroma(base).darken(-delta).hex();
}

// buildColorMapFromKeys colors each key hierarchically: the kinds are spaced
// evenly along the family's cubehelix ramp (so a handful of kinds land on
// well-separated points of it), and each "what" under a kind gets a lightness
// variant of its kind's color. Pass the keys of one family at a time (task
// keys, or blocking-reason keys) so each family gets the whole ramp to itself.
export function buildColorMapFromKeys(keys: string[], palette: Palette = "task"): ColorMap {
  const uniqueKeys = Array.from(new Set(keys)).sort();
  const scale = paletteScale(palette);

  const kinds = new Map<string, string[]>();
  for (const key of uniqueKeys) {
    const kind = kindOfKey(key);
    const group = kinds.get(kind);
    if (group) group.push(key);
    else kinds.set(kind, [key]);
  }

  const kindNames = [...kinds.keys()].sort();
  const map: ColorMap = {};
  kindNames.forEach((kind, kindIndex) => {
    const color = scale((kindIndex + 0.5) / kindNames.length).hex();
    const group = kinds.get(kind) ?? [];
    group.forEach((key, index) => {
      map[key] = shade(color, index, group.length);
    });
  });
  return map;
}

export function lookupColor(
  map: ColorMap,
  task: Task,
  mode: ColorMode = "kind-what",
): string {
  return map[taskColorKey(task, mode)] ?? "#999999";
}
