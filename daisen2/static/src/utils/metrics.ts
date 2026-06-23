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
