// Generic handling of result-directory naming: everything here comes from the
// project's herd.json `naming` (field labels, vocabularies, group_by, compare_by,
// variant_separator). Nothing is specific to one project.
import { t } from "./i18n.ts";
import type { Naming, NamingField, ProjectConfig, Run } from "./types.ts";

export type Projects = Record<string, ProjectConfig | null> | null | undefined;

/** Label of the group of runs whose names the naming rule does not parse. */
export function unparsedLabel(): string {
  return t("naming.unparsed");
}

export function namingOf(projects: Projects, project: string): Naming | null {
  const n = projects?.[project]?.naming;
  return n && Array.isArray(n.fields) ? n : null;
}

export function fieldDef(naming: Naming | null, name: string): NamingField | undefined {
  return naming?.fields.find((f) => f.name === name);
}

export function fieldLabel(naming: Naming | null, name: string): string {
  return fieldDef(naming, name)?.label || name;
}

const DATE_PATTERNS = new Set(["\\d{4}", "[0-9]{4}"]);

/** "1015" -> "10-15" for a field whose pattern is exactly four digits and the value a valid month/day. */
export function mmdd(def: NamingField | undefined, value: string): string | null {
  if (!def || !def.pattern || !DATE_PATTERNS.has(def.pattern) || !/^\d{4}$/.test(value)) return null;
  const m = Number(value.slice(0, 2)), d = Number(value.slice(2));
  const days = [31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (m < 1 || m > 12 || d < 1 || d > days[m - 1]) return null;
  return `${value.slice(0, 2)}-${value.slice(2)}`;
}

/** Display text of a field value: the vocabulary label, MM-DD for a four-digit date field, else the value. */
export function valueLabel(naming: Naming | null, name: string, value: string): string {
  const def = fieldDef(naming, name);
  return def?.values?.[value] ?? mmdd(def, value) ?? value;
}

/** The compare_by value of a run without its variant suffix, or null. */
export function compareBase(run: Run, projects: Projects): string | null {
  const naming = namingOf(projects, run.project);
  const f = naming?.compare_by;
  const v = f ? run.fields?.[f] : undefined;
  return f && v != null ? variantBase(naming, f, v) : null;
}

/**
 * Every compare_by base value in the snapshot, sorted: the index of a value in
 * this list picks its colour, so the same value has the same colour everywhere.
 */
export function compareValues(runs: Run[], projects: Projects): string[] {
  const out = new Set<string>();
  for (const r of runs) { const b = compareBase(r, projects); if (b != null) out.add(b); }
  return [...out].sort((a, b) => a.localeCompare(b));
}

/** The part of a value before the field's variant_separator ("base+mshr128" -> "base"). */
export function variantBase(naming: Naming | null, name: string, value: string): string {
  const sep = fieldDef(naming, name)?.variant_separator;
  if (!sep || !value.includes(sep)) return value;
  return value.split(sep)[0];
}

/** Identity of the "same kind" group of a run, or null when the name is not parsed. */
export function groupKey(run: Run, naming: Naming | null): string | null {
  const by = naming?.group_by;
  if (!run.fields || !by?.length) return null;
  const vals = by.map((f) => run.fields?.[f]);
  if (vals.some((v) => v == null)) return null;
  return [run.project, ...vals].join("\u0000");
}

export function groupLabel(naming: Naming | null, fields: Record<string, string>): string {
  return (naming?.group_by ?? []).map((f) => valueLabel(naming, f, fields[f] ?? "")).join(" · ");
}

/** Fields shown faintly on a row: neither grouped by nor compared by. */
export function otherFields(naming: Naming | null, fields: Record<string, string>): { name: string; value: string }[] {
  if (!naming) return [];
  const skip = new Set([...(naming.group_by ?? []), naming.compare_by ?? ""]);
  return naming.fields.filter((f) => !skip.has(f.name) && fields[f.name] != null)
    .map((f) => ({ name: f.name, value: fields[f.name] }));
}

// ---------- facets ----------

export interface FacetValue {
  value: string;
  label: string;
  count: number;
}

export interface Facet {
  id: string; // "field:<name>", "host" or "kind"
  label: string;
  values: FacetValue[];
  /** Show the raw value beside its label (vocabulary fields). */
  codes?: boolean;
}

export type FacetSel = Record<string, string[]>;

const kindLabel = (v: string) => (v === "single" || v === "campaign" || v === "sensitivity" ? t(`kind.${v}`) : v);

export function facetValueOf(run: Run, id: string, projects: Projects): string | null {
  if (id === "host") return run.host;
  if (id === "kind") return run.kind;
  if (id.startsWith("field:")) {
    const name = id.slice(6);
    const v = run.fields?.[name];
    return v == null ? null : variantBase(namingOf(projects, run.project), name, v);
  }
  return null;
}

function matchesFacets(run: Run, sel: FacetSel, projects: Projects, except?: string): boolean {
  for (const [id, vals] of Object.entries(sel)) {
    if (id === except || !vals.length) continue;
    const v = facetValueOf(run, id, projects);
    if (v == null || !vals.includes(v)) return false;
  }
  return true;
}

export function filterRuns(runs: Run[], sel: FacetSel, projects: Projects): Run[] {
  return runs.filter((r) => matchesFacets(r, sel, projects));
}

/**
 * One facet per naming field that has a vocabulary or at most `maxDistinct`
 * distinct (base) values, then host and kind. Counts are the runs that match
 * every other facet's selection.
 */
export function buildFacets(runs: Run[], projects: Projects, sel: FacetSel, maxDistinct = 12): Facet[] {
  const defs: { def: NamingField; naming: Naming }[] = [];
  const seen = new Set<string>();
  for (const p of [...new Set(runs.map((r) => r.project))]) {
    const n = namingOf(projects, p);
    for (const f of n?.fields ?? []) {
      if (!seen.has(f.name)) { seen.add(f.name); defs.push({ def: f, naming: n as Naming }); }
    }
  }
  const facets: Facet[] = [];
  const make = (id: string, label: string, labelOf: (v: string) => string, order?: string[]): Facet | null => {
    const counts = new Map<string, number>();
    const all = new Set<string>();
    for (const r of runs) {
      const v = facetValueOf(r, id, projects);
      if (v == null) continue;
      all.add(v);
      if (matchesFacets(r, sel, projects, id)) counts.set(v, (counts.get(v) ?? 0) + 1);
    }
    for (const v of sel[id] ?? []) all.add(v);
    if (!all.size) return null;
    let vals = [...all];
    if (order) vals.sort((a, b) => (order.indexOf(a) + 1 || 1e9) - (order.indexOf(b) + 1 || 1e9) || a.localeCompare(b));
    else if (vals.every((v) => /^\d+$/.test(v))) vals.sort((a, b) => Number(b) - Number(a));
    else vals.sort((a, b) => a.localeCompare(b));
    return { id, label, values: vals.map((v) => ({ value: v, label: labelOf(v), count: counts.get(v) ?? 0 })) };
  };
  for (const { def, naming } of defs) {
    const id = "field:" + def.name;
    const distinct = new Set(runs.map((r) => facetValueOf(r, id, projects)).filter((v) => v != null));
    if (!def.values && distinct.size > maxDistinct && !(sel[id] ?? []).length) continue;
    const f = make(id, def.label || def.name, (v) => valueLabel(naming, def.name, v),
      def.values ? Object.keys(def.values) : undefined);
    if (f) { f.codes = !!def.values; facets.push(f); }
  }
  const host = make("host", t("common.host"), (v) => v);
  if (host) facets.push(host);
  const kind = make("kind", t("naming.kind"), kindLabel, ["single", "campaign", "sensitivity"]);
  if (kind) facets.push(kind);
  return facets;
}

// ---------- grouping ----------

export interface RunGroup {
  key: string; // groupKey, or "" for the unparsed group
  label: string;
  project: string;
  parsed: boolean;
  runs: Run[];
  mtime: number;
}

/**
 * List items newest first: a parsed group counts as one item, an unparsed run as
 * one item. Only the first `limit` items are kept (0 = all); the kept unparsed
 * runs are gathered into one "unparsed" group placed where its newest run was.
 */
export function groupRuns(runs: Run[], projects: Projects, limit = 0): { groups: RunGroup[]; hidden: number; total: number } {
  const items: RunGroup[] = [];
  const byKey = new Map<string, RunGroup>();
  for (const r of runs) {
    const naming = namingOf(projects, r.project);
    const k = groupKey(r, naming);
    if (k == null) {
      items.push({ key: "", label: unparsedLabel(), project: r.project, parsed: false, runs: [r], mtime: r.mtime ?? 0 });
      continue;
    }
    let g = byKey.get(k);
    if (!g) {
      g = { key: k, label: groupLabel(naming, r.fields ?? {}), project: r.project, parsed: true, runs: [], mtime: 0 };
      byKey.set(k, g);
      items.push(g);
    }
    g.runs.push(r);
    g.mtime = Math.max(g.mtime, r.mtime ?? 0);
  }
  items.sort((a, b) => b.mtime - a.mtime);
  const shown = limit > 0 ? items.slice(0, limit) : items;
  const out: RunGroup[] = [];
  let unparsed: RunGroup | null = null;
  for (const it of shown) {
    if (it.parsed) {
      it.runs.sort((a, b) => (b.mtime ?? 0) - (a.mtime ?? 0));
      out.push(it);
    } else if (!unparsed) {
      unparsed = { ...it, runs: [...it.runs] };
      out.push(unparsed);
    } else unparsed.runs.push(...it.runs);
  }
  return { groups: out, hidden: items.length - shown.length, total: items.length };
}

/** Campaign runs of the same group (any host), newest first, at most `max`. */
export function groupPeers(runs: Run[], projects: Projects, key: string, withArchived: boolean, max = 4): Run[] {
  return runs
    .filter((r) => r.kind !== "single" && (withArchived || !r.archived)
      && groupKey(r, namingOf(projects, r.project)) === key)
    .sort((a, b) => (b.mtime ?? 0) - (a.mtime ?? 0))
    .slice(0, max);
}
