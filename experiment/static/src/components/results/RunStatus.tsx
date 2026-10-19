import { t } from "../../lib/i18n";
import { campaignDone, runFailed } from "../../lib/derive";
import type { Run } from "../../lib/types";
import { ProgressTrack } from "../ProgressBar";
import { exitLabel, Pill } from "../StatePill";

export type Outcome = "ok" | "failed" | "other";

/** Finished well, failed, or neither (running, or no exit code). */
export function outcomeOf(r: Run): Outcome {
  if (r.progress) return runFailed(r) ? "failed" : campaignDone(r) ? "ok" : "other";
  if (r.exit_code === 0) return "ok";
  return r.exit_code != null ? "failed" : "other";
}

/** Status of a result directory: a progress bar for campaigns, the exit code for single runs; left aligned. */
export default function RunStatus({ run }: { run: Run }) {
  if (run.progress) {
    const f = runFailed(run);
    return (
      <div className="w-full min-w-24 max-w-40">
        <div className="flex items-center gap-2 font-mono text-[11px] tabular-nums text-muted-foreground">
          <span>{run.progress.finished}/{run.progress.total ?? "?"}</span>
          {f ? <span className="text-red-600 dark:text-red-400" title={run.failures.join("\n")}>{t("progress.failed", { n: f })}</span> : null}
          {run.progress.in_progress ? <span className="ml-auto">{t("ov.inProgress", { n: run.progress.in_progress })}</span> : null}
        </div>
        <ProgressTrack bar={run.progress} failed={f} className="mt-0.5 h-1.5" />
      </div>
    );
  }
  if (run.exit_code === 0) return <Pill kind="ok">{t("state.ok")}</Pill>;
  if (run.exit_code != null) { const l = exitLabel(run.exit_code); return <Pill kind={l.kind}>{l.text}</Pill>; }
  return <Pill kind="unknown">{t("rs.noExit")}</Pill>;
}
