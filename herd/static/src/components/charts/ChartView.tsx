import { Download } from "lucide-react";
import { useMemo, useState } from "react";
import { chartCSV, flagInfo, FLAG_ORDER, type BarModel, type ChartModel } from "../../lib/charts/model";
import { t } from "../../lib/i18n";
import { chartSVG, exportSVG, seriesColor, type Theme, type Tip } from "../../lib/charts/svg";

function download(name: string, text: string, type: string) {
  const blob = new Blob([text], { type });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(a.href), 2000);
}
const safeName = (s: string) => String(s).replace(/[^\w.-]+/g, "_").slice(0, 120);

function Tooltip({ tip, x, y }: { tip: Tip; x: number; y: number }) {
  const vw = window.innerWidth, vh = window.innerHeight;
  const w = 300, h = 110;
  let left = x + 14, top = y + 14;
  if (left + w > vw - 8) left = Math.max(8, x - w - 14);
  if (top + h > vh - 8) top = Math.max(8, y - h - 14);
  return (
    <div className="pointer-events-none fixed z-30 max-w-[min(300px,calc(100vw-16px))] rounded-md border bg-card px-2.5 py-1.5 text-xs leading-normal shadow-lg" style={{ left, top }}>
      <div className={`font-bold tabular-nums ${tip.crit ? "text-sm text-red-600 dark:text-red-400" : "text-[15px]"}`}>{tip.v}</div>
      <div><span className="mr-1.5 inline-block h-0.5 w-3 align-middle" style={{ background: tip.sw }} />{tip.name}</div>
      {tip.lines.map((l, i) => <div key={i} className="text-muted-foreground">{l}</div>)}
    </div>
  );
}

/** One SVG chart with hover/focus tooltips on its hit targets. */
export function SvgChart({ m, th, width, horiz }: { m: ChartModel; th: Theme; width: number; horiz: boolean }) {
  const { svg, tips } = useMemo(() => {
    const t: Tip[] = [];
    return { svg: chartSVG(m, th, width, 0, t, horiz), tips: t };
  }, [m, th, width, horiz]);
  const [hover, setHover] = useState<{ tip: Tip; x: number; y: number } | null>(null);
  const tipOf = (el: Element | null) => {
    const a = el?.closest?.("[data-tip]")?.getAttribute("data-tip");
    return a ? tips[Number(a.split(":")[1])] ?? null : null;
  };
  return (
    <>
      <div className="herd-chart" dangerouslySetInnerHTML={{ __html: svg }}
        onPointerMove={(e) => { const t = tipOf(e.target as Element); setHover(t ? { tip: t, x: e.clientX, y: e.clientY } : null); }}
        onPointerLeave={() => setHover(null)}
        onFocus={(e) => {
          const t = tipOf(e.target as Element);
          if (!t) return;
          const r = (e.target as Element).getBoundingClientRect();
          setHover({ tip: t, x: r.left + r.width / 2, y: r.top + 10 });
        }}
        onBlur={() => setHover(null)} />
      {hover ? <Tooltip {...hover} /> : null}
    </>
  );
}

function Legend({ m, th, hide, onToggle }: { m: BarModel; th: Theme; hide: string[]; onToggle: (k: string) => void }) {
  if (!m.series.length || (m.visible.length === m.series.length && (!m.legend || m.series.length < 2))) return null;
  return (
    <div className="my-2 flex flex-wrap gap-x-3.5 gap-y-0.5 text-xs">
      {m.series.map((s) => {
        const off = hide.includes(s.hideKey);
        return (
          <button key={s.hideKey} type="button" aria-pressed={!off} onClick={() => onToggle(s.hideKey)}
            title={(off ? t("view.show") : t("view.hide")) + (s.long ? "\n" + s.long : "")}
            className={`inline-flex items-center gap-1.5 py-0.5 ${off ? "text-muted-foreground line-through" : ""}`}>
            <span className={`inline-block h-3 w-3 shrink-0 rounded-sm ${off ? "opacity-25" : ""}`} style={{ background: seriesColor(th, s, m.series.length === 1) }} />
            {s.label}
          </button>
        );
      })}
    </div>
  );
}

function Marks({ m }: { m: BarModel }) {
  const items = FLAG_ORDER.filter((f) => m.flagCounts[f]).map((f) => {
    const fi = flagInfo(f, m.spec.match);
    return <span key={f}><b className={`mr-0.5 ${fi?.crit ? "text-red-600 dark:text-red-400" : ""}`}>{fi?.glyph}</b>{fi?.text ?? f} {m.flagCounts[f]}</span>;
  });
  if (m.cap && m.visible.some((s) => (s.gmean ?? 0) > m.cap || m.workloads.some((w) => s.pairs[w].flag === "ok" && (s.pairs[w].s as number) > m.cap))) {
    items.push(<span key="cap">{t("view.capped", { cap: m.cap })}</span>);
  }
  return items.length ? <div className="mt-1 flex flex-wrap gap-x-3.5 gap-y-0.5 text-xs text-muted-foreground">{items}</div> : null;
}

function DataTable({ m }: { m: BarModel }) {
  return (
    <div className="overflow-x-auto">
      <table className="herd-table herd-table-sm">
        <thead><tr><th>{m.spec.x}</th>{m.visible.map((s) => <th key={s.hideKey}>{s.label}</th>)}</tr></thead>
        <tbody>
          {m.workloads.map((w) => (
            <tr key={w}>
              <td>{m.labels[w] || w} {w !== (m.labels[w] || w) ? <span className="text-muted-foreground">{w}</span> : null}</td>
              {m.visible.map((s) => {
                const p = s.pairs[w], fi = flagInfo(p.flag, m.spec.match);
                return p.flag === "ok"
                  ? <td key={s.hideKey} className="font-mono tabular-nums">{(p.s as number).toFixed(3)}</td>
                  : <td key={s.hideKey} className={fi?.crit ? "text-red-600 dark:text-red-400" : "text-muted-foreground"} title={p.note || ""}>
                      {fi?.glyph || ""} {fi?.text || p.flag}</td>;
              })}
            </tr>
          ))}
          {m.summary ? (
            <tr>
              <td><b>GMEAN</b> <span className="text-muted-foreground">n={m.common.length}</span></td>
              {m.visible.map((s) => <td key={s.hideKey} className="font-mono font-bold tabular-nums">{s.gmean == null ? "—" : s.gmean.toFixed(3)}</td>)}
            </tr>
          ) : null}
        </tbody>
      </table>
    </div>
  );
}

function DlButton({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} className="inline-flex items-center gap-1 rounded border bg-card px-2 py-0.5 text-xs hover:bg-muted">
      <Download className="h-3.5 w-3.5" />{children}
    </button>
  );
}

/** One chart card: title, notes, legend (click to hide), SVG, marks, downloads and data table. */
export default function ChartView({ m, th, width, hide, onToggleHide }: {
  m: BarModel; th: Theme; width: number; hide: string[]; onToggleHide: (k: string) => void;
}) {
  const line = m.line;
  const wide = width >= 900;
  const bwid = line ? (wide ? Math.floor((width - 14) * 0.72) : width) : width;
  const lwid = line ? (wide ? width - 14 - bwid : Math.min(width, 520)) : 0;
  return (
    <section className="border-t p-4 first:border-t-0">
      <h3 className="break-words text-base font-semibold">{m.title}</h3>
      {m.sub.map((t, i) => <div key={i} className="mt-0.5 text-xs text-muted-foreground">{t}</div>)}
      {m.warn.map((t, i) => <div key={"w" + i} className="mt-0.5 break-words text-xs text-amber-700 dark:text-amber-300">{t}</div>)}
      <Legend m={m} th={th} hide={hide} onToggle={onToggleHide} />
      {line ? (
        <div className={`grid items-start gap-3.5 ${wide ? "grid-cols-[minmax(0,72fr)_minmax(0,28fr)]" : "grid-cols-1"}`}>
          <div>
            <h4 className="mb-1 mt-1.5 text-xs font-semibold text-muted-foreground">{t("view.perX", { x: m.spec.x })}</h4>
            {m.visible.length ? <SvgChart m={m} th={th} width={bwid} horiz={bwid < 560} /> : null}
            <Marks m={m} />
          </div>
          <div>
            <h4 className="mb-1 mt-1.5 text-xs font-semibold text-muted-foreground">{t("view.trend", { n: m.common.length, x: m.spec.x })}</h4>
            {m.visible.length ? <SvgChart m={line} th={th} width={lwid} horiz={false} /> : null}
          </div>
        </div>
      ) : (
        <>
          {m.visible.length ? <SvgChart m={m} th={th} width={width} horiz={width < 560} /> : null}
          <Marks m={m} />
        </>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-2">
        {line ? <>
          <DlButton onClick={() => download(safeName(m.id) + ".svg", exportSVG(m), "image/svg+xml")}>{t("view.dlBars")}</DlButton>
          <DlButton onClick={() => download(safeName(line.id) + ".svg", exportSVG(line), "image/svg+xml")}>{t("view.dlLine")}</DlButton>
        </> : <DlButton onClick={() => download(safeName(m.id) + ".svg", exportSVG(m), "image/svg+xml")}>{t("view.dlSvg")}</DlButton>}
        <DlButton onClick={() => download(safeName(m.id) + ".csv", chartCSV(m), "text/csv")}>{t("view.dlCsv")}</DlButton>
        <details className="basis-full text-xs">
          <summary className="cursor-pointer text-muted-foreground">{t("view.table")}</summary>
          <DataTable m={m} />
        </details>
        {m.diffs?.length ? (
          <details className="basis-full text-xs">
            <summary className="cursor-pointer text-muted-foreground">{t("view.diffs", { x: m.spec.x })}</summary>
            {m.diffs.map((t, i) => <div key={i} className="mt-0.5 break-words text-muted-foreground">{t}</div>)}
          </details>
        ) : null}
      </div>
    </section>
  );
}
