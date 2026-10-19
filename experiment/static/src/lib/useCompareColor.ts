import { useMemo } from "react";
import { THEMES } from "./charts/svg";
import { compareBase, compareValues, fieldDef, namingOf } from "./naming";
import { useStatusState } from "./status";
import type { Run } from "./types";
import { useDark } from "./useDark";

export interface CompareColor { color: string; variant: boolean; base: string }

/**
 * Colour of a run's compare_by value (e.g. the config "base"): the categorical
 * chart palette indexed by the value's place among all values in the snapshot,
 * so the same value has the same colour on every page. Variants ("base+x")
 * keep the base colour and are marked `variant`.
 */
export function useCompareColor(): (run: Run) => CompareColor | null {
  const { status } = useStatusState();
  const dark = useDark();
  return useMemo(() => {
    const values = status ? compareValues(status.runs, status.projects) : [];
    const pal = THEMES[dark ? "dark" : "light"].categorical;
    return (run: Run) => {
      const base = status ? compareBase(run, status.projects) : null;
      if (base == null) return null;
      const i = values.indexOf(base);
      const naming = namingOf(status?.projects, run.project);
      const f = naming?.compare_by ?? "";
      const sep = fieldDef(naming, f)?.variant_separator;
      const full = run.fields?.[f] ?? "";
      return { color: pal[(i < 0 ? 0 : i) % pal.length], variant: !!sep && full.includes(sep), base };
    };
  }, [status, dark]);
}
