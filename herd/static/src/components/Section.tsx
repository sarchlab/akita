import type { ReactNode } from "react";

/** Akita-style section header: text-base title, muted one-line subtitle, optional right side. */
export function SectionHeader({ title, subtitle, right }: { title: ReactNode; subtitle?: ReactNode; right?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <div className="text-base font-semibold leading-tight">{title}</div>
        {subtitle ? <div className="mt-1 text-xs text-muted-foreground">{subtitle}</div> : null}
      </div>
      {right ? <div className="flex shrink-0 items-center gap-2">{right}</div> : null}
    </div>
  );
}

/** A bordered block of a detail pane. */
export function Section({ title, subtitle, right, children }: { title: ReactNode; subtitle?: ReactNode; right?: ReactNode; children: ReactNode }) {
  return (
    <section className="border-b p-4 last:border-b-0">
      <SectionHeader title={title} subtitle={subtitle} right={right} />
      <div className="mt-3">{children}</div>
    </section>
  );
}

/** Empty state: what is missing, and one line on how to get something here. */
export function EmptyState({ title, hint, className = "p-10" }: { title: ReactNode; hint?: ReactNode; className?: string }) {
  return (
    <div className={`text-center ${className}`}>
      <div className="text-sm font-medium text-muted-foreground">{title}</div>
      {hint ? <div className="mt-1 text-xs text-muted-foreground/80">{hint}</div> : null}
    </div>
  );
}
