import { t } from "../lib/i18n";
import { groupLabel, otherFields, type Projects, namingOf, valueLabel } from "../lib/naming";
import type { Run } from "../lib/types";
import { useCompareColor } from "../lib/useCompareColor";

/** The compare_by value as a chip in its stable colour; variants get a dashed border. */
export function CompareChip({ run, value }: { run: Run; value: string }) {
  const c = useCompareColor()(run);
  return (
    <span className={`inline-block whitespace-nowrap rounded border px-1.5 font-mono text-xs font-semibold ${c?.variant ? "border-dashed" : ""}`}
      style={c ? { borderColor: c.color, background: `${c.color}1f`, boxShadow: `inset 3px 0 0 ${c.color}` } : undefined}
      title={c?.variant ? t("run.variantOf", { value, base: c.base }) : value}>
      {value}
    </span>
  );
}

/**
 * A result directory name through the project's naming: the compare_by value as
 * a chip, the other fields faint (and the group label first when `withGroup`).
 * Unparsed names are shown raw.
 */
export default function RunName({ run, projects, withGroup = false }: { run: Run; projects: Projects; withGroup?: boolean }) {
  const naming = namingOf(projects, run.project);
  const f = run.fields;
  if (!f || !naming) return <span className="break-all font-mono text-xs" title={run.name}>{run.name}</span>;
  const cmp = naming.compare_by ? f[naming.compare_by] : undefined;
  return (
    <span className="inline-flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-0.5" title={run.name}>
      {withGroup ? <span className="text-sm font-semibold">{groupLabel(naming, f)}</span> : null}
      {cmp != null ? <CompareChip run={run} value={cmp} /> : null}
      {otherFields(naming, f).map((o) => (
        <span key={o.name} className="whitespace-nowrap font-mono text-[11px] text-muted-foreground">{valueLabel(naming, o.name, o.value)}</span>
      ))}
    </span>
  );
}
