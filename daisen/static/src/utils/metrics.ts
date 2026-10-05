// The selectable Y-axis metrics and their human labels, shared by the dashboard's
// axis menus and the components widget's per-chart legend / auto-selection.
export const AXIS_OPTIONS = [
  { value: "ReqInCount", label: "Incoming Request Rate" },
  { value: "ReqCompleteCount", label: "Request Complete Rate" },
  { value: "AvgLatency", label: "Average Request Latency" },
  { value: "ConcurrentTask", label: "Number Concurrent Task" },
  { value: "RequestBufferPressure", label: "Request Buffer Pressure" },
  { value: "ResponseBufferPressure", label: "Response Buffer Pressure" },
  { value: "PendingReqOut", label: "Pending Request Out" },
  { value: "-", label: " - " },
];

// axisLabel maps a metric key to its human label (falling back to the key).
export function axisLabel(value: string): string {
  return AXIS_OPTIONS.find((option) => option.value === value)?.label ?? value;
}

// Per-metric line colors, so a given metric reads as the same color everywhere and
// two different metrics (the two axes of one chart, or different auto-selected
// charts in the components widget) are visually distinct. The long-standing
// defaults keep red (incoming rate) and blue (latency); the rest are a categorical
// palette. Unknown / "-" falls back to grey.
const METRIC_COLORS: Record<string, string> = {
  ReqInCount: "#d7191c",
  ReqCompleteCount: "#4daf4a",
  AvgLatency: "#2c7bb6",
  ConcurrentTask: "#984ea3",
  RequestBufferPressure: "#a65628",
  ResponseBufferPressure: "#ff7f00",
  PendingReqOut: "#f781bf",
};

// axisColor maps a metric key to its line color.
export function axisColor(value: string): string {
  return METRIC_COLORS[value] ?? "#999999";
}
