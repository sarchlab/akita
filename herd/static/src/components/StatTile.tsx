import type { ReactNode } from "react";

/** An Akita-style stat tile; `tone` colours the value (only use it when the value is not zero). */
export default function StatTile({ label, value, sub, tone, title }: {
  label: string; value: ReactNode; sub?: ReactNode; tone?: "amber" | "red"; title?: string;
}) {
  const accent = tone === "amber" ? "border-l-amber-500 text-amber-600 dark:text-amber-400"
    : tone === "red" ? "border-l-red-500 text-red-600 dark:text-red-400" : "border-l-transparent";
  return (
    <div className={`rounded-lg border border-l-4 bg-card px-4 py-3 shadow-xs ${accent}`} title={title}>
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className={`mt-1 font-mono text-2xl font-semibold tabular-nums ${tone ? "" : "text-foreground"}`}>{value}</div>
      {sub ? <div className="mt-0.5 truncate text-xs text-muted-foreground">{sub}</div> : null}
    </div>
  );
}
