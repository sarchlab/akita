import { t } from "../lib/i18n";
import { stateLabel } from "../lib/notify";

export const PILL: Record<string, string> = {
  running: "bg-sky-600 text-white",
  stalled: "bg-amber-500 text-white",
  ok: "bg-emerald-600 text-white",
  failed: "bg-red-600 text-white",
  killed: "bg-amber-700 text-white",
  died: "bg-amber-700 text-white",
  stopped: "bg-slate-500 text-white",
  queued: "bg-violet-600 text-white",
  unknown: "bg-slate-400 text-white dark:bg-slate-600",
};

export function Pill({ kind, children, title }: { kind: string; children: React.ReactNode; title?: string }) {
  return (
    <span title={title} className={`inline-block whitespace-nowrap rounded-full px-2 py-px text-xs font-semibold ${PILL[kind] ?? PILL.unknown}`}>
      {children}
    </span>
  );
}

export function exitLabel(code: number): { kind: string; text: string } {
  if (code === 143 || code === 137) return { kind: "killed", text: t("pill.killed", { code }) };
  return { kind: "failed", text: t("pill.failed", { code }) };
}

export default function StatePill({ state, exitCode }: { state: string; exitCode?: number | null }) {
  if (state === "failed" && exitCode != null) {
    const l = exitLabel(exitCode);
    return <Pill kind={l.kind}>{l.text}</Pill>;
  }
  return <Pill kind={state}>{stateLabel(state)}</Pill>;
}

export function ArchivedTag() {
  return (
    <span className="ml-1.5 inline-block whitespace-nowrap rounded border border-slate-300 px-1 text-[11px] text-muted-foreground dark:border-slate-600" title={t("pill.archivedHint")}>
      {t("pill.archived")}
    </span>
  );
}
