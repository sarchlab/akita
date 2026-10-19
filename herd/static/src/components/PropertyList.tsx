import { t } from "../lib/i18n";
import { fmtTimeLike } from "../lib/format";
import type { Property } from "../lib/types";
import { SectionHeader } from "./Section";

/** Akita's exec_info list: property names left, values in mono; times shown in local time. */
export default function PropertyList({ entries, title, source }: { entries: Property[]; title: string; source: string }) {
  return (
    <section>
      <SectionHeader title={title} subtitle={source}
        right={<span className="font-mono text-xs text-muted-foreground">{t("common.items", { n: entries.length })}</span>} />
      {entries.length ? (
        <dl className="mt-2 divide-y">
          {entries.map((e, i) => {
            const t = fmtTimeLike(e.value);
            return (
              <div key={`${e.property}-${i}`} className="grid gap-1 py-1.5 text-sm sm:grid-cols-[9rem_minmax(0,1fr)] sm:gap-2">
                <dt className="font-medium text-muted-foreground">{e.property}</dt>
                <dd className="min-w-0 break-all font-mono text-sm text-foreground" title={t ? e.value : undefined}>{t ?? e.value}</dd>
              </div>
            );
          })}
        </dl>
      ) : (
        <div className="mt-2 border-t pt-2 text-sm text-muted-foreground">{t("common.noProps")}</div>
      )}
    </section>
  );
}
