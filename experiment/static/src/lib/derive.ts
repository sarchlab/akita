// Small derived values over the status snapshot.
import type { Run, Session } from "./types.ts";

export const isActive = (s: Session) => s.state === "running" || s.state === "stalled";

/** Running, stalled, or waiting in a SLURM queue. */
export const isLive = (s: Session) => isActive(s) || s.state === "queued";

/** Last change of a session: log mtime when the server sends it, else last output / end / start. */
export function sessionMtime(s: Session): number {
  return s.mtime ?? s.last_output ?? s.end ?? s.start ?? 0;
}

/** Failed cells of a campaign (exact count when the server sends it; the failure list is capped). */
export function runFailed(r: Run): number {
  return r.failed ?? r.failures?.length ?? 0;
}

/** Whether a run is a campaign (plain or sensitivity sweep). */
export function campaignRun(r: Run): boolean {
  return r.kind === "campaign" || r.kind === "sensitivity";
}

export function campaignDone(r: Run): boolean {
  return !!r.progress && r.progress.total != null && r.progress.finished >= r.progress.total;
}
