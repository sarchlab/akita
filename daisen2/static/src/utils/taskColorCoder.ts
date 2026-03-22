import type { Task } from "../types/task";

/**
 * A simple, deterministic colour palette generated from cubehelix-like hues.
 * We avoid depending on chroma-js by using an HSL-based palette.
 */
const PALETTE = [
  "#1f77b4",
  "#ff7f0e",
  "#2ca02c",
  "#d62728",
  "#9467bd",
  "#8c564b",
  "#e377c2",
  "#7f7f7f",
  "#bcbd22",
  "#17becf",
  "#aec7e8",
  "#ffbb78",
  "#98df8a",
  "#ff9896",
  "#c5b0d5",
  "#c49c94",
  "#f7b6d2",
  "#c7c7c7",
  "#dbdb8d",
  "#9edae5",
];

export interface ColorMap {
  [kindWhat: string]: string;
}

/**
 * Builds a deterministic colour map from an array of tasks.
 * Each unique "kind-what" combo gets a colour.
 */
export function buildColorMap(tasks: Task[]): ColorMap {
  const seen = new Set<string>();
  for (const t of tasks) {
    seen.add(`${t.kind}-${t.what}`);
  }

  const keys = Array.from(seen).sort();
  const map: ColorMap = {};
  keys.forEach((k, i) => {
    map[k] = PALETTE[i % PALETTE.length];
  });
  return map;
}

export function lookupColor(map: ColorMap, task: Task): string {
  const key = `${task.kind}-${task.what}`;
  return map[key] ?? "#999";
}
