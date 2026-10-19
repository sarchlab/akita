import { t } from "../../lib/i18n";
import { Search, X } from "lucide-react";
import type { Facet, FacetSel } from "../../lib/naming";
import Toggle from "../Toggle";
import { Input } from "../ui/input";

/** The results sidebar: search, one facet per naming field (plus host and kind), archived toggle. */
export default function Facets({ facets, sel, onSel, search, onSearch, showArchived, onArchived, archivedCount, total }: {
  facets: Facet[]; sel: FacetSel; onSel: (s: FacetSel) => void; search: string; onSearch: (v: string) => void;
  showArchived: boolean; onArchived: (v: boolean) => void; archivedCount: number; total: number;
}) {
  const active = Object.values(sel).some((v) => v.length);
  const toggle = (id: string, v: string) => {
    const cur = sel[id] ?? [];
    const next = cur.includes(v) ? cur.filter((x) => x !== v) : [...cur, v];
    const out = { ...sel, [id]: next };
    if (!next.length) delete out[id];
    onSel(out);
  };
  return (
    <div className="flex flex-col">
      <div className="border-b p-3">
        <div className="mb-2 flex items-center gap-2">
          <Search className="h-4 w-4 text-muted-foreground" />
          <Input value={search} placeholder={t("res.search")} onChange={(e) => onSearch(e.target.value)} />
        </div>
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span>{t("res.total", { n: total })}</span>
          {active ? (
            <button type="button" className="inline-flex items-center gap-1 text-primary hover:underline" onClick={() => onSel({})}>
              <X className="h-3 w-3" />{t("res.clearFilters")}
            </button>
          ) : null}
        </div>
      </div>
      {facets.map((f) => (
        <div key={f.id} className="border-b px-3 py-2">
          <div className="mb-1 text-xs font-semibold text-muted-foreground">{f.label}</div>
          <div className="flex flex-col">
            {f.values.map((v) => {
              const on = (sel[f.id] ?? []).includes(v.value);
              return (
                <label key={v.value} className={`flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-muted/70 ${v.count ? "" : "text-muted-foreground"}`}>
                  <input type="checkbox" className="h-3.5 w-3.5 accent-primary" checked={on} onChange={() => toggle(f.id, v.value)} />
                  <span className="min-w-0 flex-1 truncate" title={v.value}>{v.label}</span>
                  {f.codes && v.label !== v.value ? <span className="font-mono text-[10px] text-muted-foreground">{v.value}</span> : null}
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">{v.count}</span>
                </label>
              );
            })}
          </div>
        </div>
      ))}
      <div className="p-3">
        <Toggle checked={showArchived} onChange={onArchived} title={t("res.archivedHint")}>
          {t("common.showArchived", { n: archivedCount })}
        </Toggle>
      </div>
    </div>
  );
}
