import { t } from "../lib/i18n";
import type { ProgressBar as Bar } from "../lib/types";

const clamp = (v: number) => Math.min(1, Math.max(0, v));

/** Akita's two-tone progress bar: finished sky-600, in progress sky-300, rest slate-200. */
export function ProgressTrack({ bar, failed = 0, className = "h-3" }: { bar: Bar; failed?: number; className?: string }) {
  // Without a total (an open-ended job) the bar shows only that work is in progress.
  const total = bar.total || 0;
  const finished = total ? clamp(bar.finished / total) : 0;
  const active = total ? clamp((bar.finished + bar.in_progress) / total) : bar.in_progress ? 1 : 0;
  const share = active ? finished / active : 0;
  const failedShare = bar.finished ? clamp(failed / bar.finished) : 0;
  return (
    <div className={`${className} overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700`}>
      <div className="h-full bg-sky-300 dark:bg-sky-800" style={{ width: `${active * 100}%` }}>
        <div className="flex h-full justify-end bg-sky-600 dark:bg-sky-500" style={{ width: `${share * 100}%` }}>
          {failedShare > 0 ? <div className="h-full bg-red-500" style={{ width: `${failedShare * 100}%` }} title={t("progress.failed", { n: failed })} /> : null}
        </div>
      </div>
    </div>
  );
}

export default function ProgressBar({ bar, label, failed = 0, compact = false, extra }: {
  bar: Bar; label?: React.ReactNode; failed?: number; compact?: boolean; extra?: React.ReactNode;
}) {
  return (
    <div className="min-w-0">
      <div className={`${compact ? "mb-1" : "mb-2"} flex items-center justify-between gap-3 text-sm`}>
        <div className="min-w-0 truncate font-semibold" title={bar.name}>{label ?? bar.name}</div>
        <div className="shrink-0 font-mono text-xs text-muted-foreground">
          {bar.finished}/{bar.total ?? "?"}
        </div>
      </div>
      <ProgressTrack bar={bar} failed={failed} className={compact ? "h-2" : "h-3"} />
      {compact ? (extra ?? null) : (
        <div className="mt-2 grid grid-cols-3 gap-3 text-xs text-muted-foreground">
          <span>{t("progress.finished", { n: bar.finished })}{failed ? <span className="text-red-600 dark:text-red-400">{t("progress.failedParen", { n: failed })}</span> : null}</span>
          <span>{t("progress.inProgress", { n: bar.in_progress })}</span>
          <span>{t("progress.total", { n: bar.total ?? "?" })}</span>
        </div>
      )}
      {!compact && extra ? extra : null}
    </div>
  );
}
