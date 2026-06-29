import * as d3 from "d3";
import type { Segment } from "../../types/task";

// Single source of truth for the chart visual language shared by the component
// view and the task view. Bars, milestone waves, and time axes on both pages read
// their colors, opacities, and geometry from here, so the same semantic state
// always renders identically and can't drift between the two pages.

// --- Colors ---
export const COLOR_AXIS_LABEL = "#475569"; // tick labels
export const COLOR_GRID = "#000"; // gridlines, tick marks, baselines
export const COLOR_BAR_STROKE = "#000000";
export const COLOR_TASK_FALLBACK = "#9ca3af"; // bar/milestone color when unmapped
export const COLOR_HALO = "#ffffff"; // section-label + milestone-node halo

// --- Axis ---
export const AXIS_TICK_COUNT = 12;
export const AXIS_LABEL_FONT_SIZE = 10;
export const GRID_DASH = "3,3";
export const GRID_OPACITY = 0.3;

// --- Section labels ---
export const SECTION_LABEL_FONT_SIZE = 12;
export const SECTION_LABEL_HALO_WIDTH = 3;

// --- Affordance opacities ---
export const STROKE_OPACITY_SELECTED = 0.8;
export const STROKE_OPACITY_DEFAULT = 0.2;
const OPACITY_DIM_HIGHLIGHT = 0.4; // non-matching bar when a legend key is highlighted
const OPACITY_DIM_SELECTION = 0.6; // non-selected bar when a task is selected
export const OPACITY_DIM_MILESTONE = 0.25; // non-selected / non-matching milestone wave

// --- Milestone glyph geometry ---
export const MILESTONE_WAVE_WIDTH = 1.5;
export const MILESTONE_WAVE_WIDTH_SELECTED = 3;
export const MILESTONE_DOT_R = 3;
export const MILESTONE_DOT_R_SELECTED = 3.5;
export const MILESTONE_RING_R = 6;
export const MILESTONE_HIT_HEIGHT = 16; // transparent click target around the wave

// The one fill-opacity formula every task bar uses: a legend highlight dims
// non-matching bars; otherwise a selection dims the non-selected ones.
export function barOpacity(state: { selected: boolean; highlighted: boolean; hasHighlight: boolean; hasSelection: boolean }): number {
  if (state.hasHighlight) return state.highlighted ? 1 : OPACITY_DIM_HIGHLIGHT;
  if (state.hasSelection && !state.selected) return OPACITY_DIM_SELECTION;
  return 1;
}

// The one stroke-opacity formula every task bar uses: a darker border marks the
// selected bar (or the highlighted one).
export function barStrokeOpacity(state: { selected: boolean; highlighted: boolean; hasHighlight: boolean }): number {
  return state.selected || (state.hasHighlight && state.highlighted) ? STROKE_OPACITY_SELECTED : STROKE_OPACITY_DEFAULT;
}

// d3 scale lookup guarded against NaN (shared by every chart).
export function safeScale(scale: d3.ScaleLinear<number, number>, value: number): number {
  return scale(value) ?? 0;
}

// The "no trace collected" spans within [startTime, endTime] — the inverse of the
// recorded segments — drawn as the gap hatch on every chart.
export function gapSegments(segments: Segment[], startTime: number, endTime: number): Segment[] {
  if (!segments.length) return [];
  const sorted = [...segments].sort((a, b) => a.start_time - b.start_time);
  const gaps: Segment[] = [];
  if (sorted[0].start_time > startTime) {
    gaps.push({ start_time: startTime, end_time: Math.min(sorted[0].start_time, endTime) });
  }
  for (let index = 0; index < sorted.length - 1; index++) {
    const start = Math.max(sorted[index].end_time, startTime);
    const end = Math.min(sorted[index + 1].start_time, endTime);
    if (start < end) gaps.push({ start_time: start, end_time: end });
  }
  const last = sorted[sorted.length - 1];
  if (last.end_time < endTime) {
    gaps.push({ start_time: Math.max(last.end_time, startTime), end_time: endTime });
  }
  return gaps;
}
