/** A labelled usage bar (host load, memory, GPU); red above 85 %. */
export default function Meter({ label, frac, text }: { label: string; frac: number; text: string }) {
  const pct = Math.max(0, Math.min(1, frac || 0)) * 100;
  return (
    <div className="grid grid-cols-[4rem_minmax(0,1fr)_auto] items-center gap-2 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <div className="h-2 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
        <div className={`h-full ${pct > 85 ? "bg-red-500" : "bg-sky-600 dark:bg-sky-500"}`} style={{ width: `${pct.toFixed(1)}%` }} />
      </div>
      <span className="whitespace-nowrap font-mono tabular-nums">{text}</span>
    </div>
  );
}
