export interface TimeRange {
  startTime: number;
  endTime: number;
}

// A kind-what blocking-reason key for a hardware resource is
// "hardware_resource-<what>"; the suffix is the resource name.
export const HW_RESOURCE_PREFIX = "hardware_resource-";

export function resourceHrefForWhat(
  what: string,
  resourceRange?: TimeRange,
): string {
  const params = new URLSearchParams({ what });
  if (resourceRange) {
    params.set("starttime", String(resourceRange.startTime));
    params.set("endtime", String(resourceRange.endTime));
  }
  return `/resource?${params.toString()}`;
}

export function resourceWhatFromReason(reason: string): string | null {
  return reason.startsWith(HW_RESOURCE_PREFIX)
    ? reason.slice(HW_RESOURCE_PREFIX.length)
    : null;
}

export function resourceHrefForReason(
  reason: string,
  resourceRange?: TimeRange,
): string | null {
  const what = resourceWhatFromReason(reason);
  return what ? resourceHrefForWhat(what, resourceRange) : null;
}
