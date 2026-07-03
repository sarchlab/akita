import chroma from "chroma-js";
import type { Task } from "../types/task";

export type ColorMap = Record<string, string>;

// How tasks are grouped for coloring (and for the task-count chart's bands):
// by their kind alone, or by the finer "kind-what" pair. Must stay in sync with
// the server's component_timeline `group` param so a band's key matches the key
// a task computes here.
export type ColorMode = "kind" | "kind-what";

// Tasks and blocking-reason milestones draw from two separate categorical
// palettes so the two legends read as different families.
export type Palette = "task" | "milestone";

export function taskColorKey(
  task: Pick<Task, "kind" | "what">,
  mode: ColorMode = "kind-what",
): string {
  return mode === "kind" ? task.kind : `${task.kind}-${task.what}`;
}

// Tableau 10 — categorical, tuned for adjacent-band distinguishability.
const TASK_PALETTE = [
  "#4e79a7",
  "#f28e2c",
  "#e15759",
  "#76b7b2",
  "#59a14f",
  "#edc949",
  "#af7aa1",
  "#ff9da7",
  "#9c755f",
  "#bab0ab",
];

// ColorBrewer Dark2 — darker and more saturated than the task family, so
// blocking-reason waves and dots stand apart from task bars.
const MILESTONE_PALETTE = [
  "#1b9e77",
  "#d95f02",
  "#7570b3",
  "#e7298a",
  "#66a61e",
  "#e6ab02",
  "#a6761d",
  "#666666",
];

const PALETTES: Record<Palette, string[]> = {
  task: TASK_PALETTE,
  milestone: MILESTONE_PALETTE,
};

// FNV-1a. Hashing the kind (rather than indexing into the sorted key set) keeps
// a kind's hue stable across views, zoom levels, and legend-set changes.
function hashString(value: string): number {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i++) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

// Keys are either a bare kind or the server's `Kind || '-' || What`, so the
// text before the first "-" is the kind either way.
function kindOfKey(key: string): string {
  const dash = key.indexOf("-");
  return dash === -1 ? key : key.slice(0, dash);
}

// shade spreads a kind's "what" variants around the base color by lightness,
// keeping the hue so the variants still read as one family.
function shade(base: string, index: number, count: number): string {
  if (count <= 1) return base;
  const span = Math.min(2.2, 0.7 * (count - 1));
  const delta = -span / 2 + (index / (count - 1)) * span;
  return delta >= 0 ? chroma(base).brighten(delta).hex() : chroma(base).darken(-delta).hex();
}

// buildColorMapFromKeys colors each key hierarchically: the kind picks the hue
// from a categorical palette (by stable hash, probing past hues already taken
// in this view), and each "what" under a kind gets a lightness variant of that
// hue. Pass the keys of one family at a time (task keys, or blocking-reason
// keys) so each family stays within its own palette.
export function buildColorMapFromKeys(keys: string[], palette: Palette = "task"): ColorMap {
  const uniqueKeys = Array.from(new Set(keys)).sort();
  const base = PALETTES[palette];

  const kinds = new Map<string, string[]>();
  for (const key of uniqueKeys) {
    const kind = kindOfKey(key);
    const group = kinds.get(kind);
    if (group) group.push(key);
    else kinds.set(kind, [key]);
  }

  const kindColor = new Map<string, string>();
  const taken = new Set<number>();
  for (const kind of [...kinds.keys()].sort()) {
    let slot = hashString(kind) % base.length;
    if (taken.size < base.length) {
      while (taken.has(slot)) slot = (slot + 1) % base.length;
      taken.add(slot);
    }
    kindColor.set(kind, base[slot]);
  }

  const map: ColorMap = {};
  for (const [kind, group] of kinds) {
    const color = kindColor.get(kind) ?? base[0];
    group.forEach((key, index) => {
      map[key] = shade(color, index, group.length);
    });
  }
  return map;
}

export function lookupColor(
  map: ColorMap,
  task: Task,
  mode: ColorMode = "kind-what",
): string {
  return map[taskColorKey(task, mode)] ?? "#999999";
}
