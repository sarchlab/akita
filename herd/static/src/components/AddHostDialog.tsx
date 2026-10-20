import { useState, type FormEvent, type ReactNode } from "react";
import { Loader2, X } from "lucide-react";
import { Button } from "./ui/button";
import { addHost, entryForm, entryFrom, launcherRun, scanHost, type EntryForm, type ScanForm } from "../lib/hosts";
import { refreshNow } from "../lib/status";
import { fmtDur } from "../lib/format";
import { t } from "../lib/i18n";
import type { HostScanResponse, Unmanaged } from "../lib/types";

const inputClass = "h-8 w-full rounded-md border border-input bg-background px-2 text-sm outline-hidden focus-visible:ring-1 focus-visible:ring-ring";

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-xs">
      <span className="font-medium">{label}</span>
      {children}
      {hint ? <span className="text-muted-foreground">{hint}</span> : null}
    </label>
  );
}

/** The user's simulations that no Herd session started, as a table. */
export function UnmanagedList({ items, now }: { items: Unmanaged[]; now: number }) {
  return (
    <div className="flex flex-col divide-y rounded-md border text-xs">
      {items.map((u) => (
        <div key={u.pid} className="flex flex-col gap-0.5 px-2 py-1.5">
          <div className="flex flex-wrap items-baseline gap-x-2">
            <span className="font-semibold">{launcherRun(u.launcher) || u.exe}</span>
            <span className="font-mono text-muted-foreground">pid {u.pid} · {u.cpu.toFixed(0)}% CPU · {u.rss_gb.toFixed(1)} GB · {fmtDur(now - u.start)}</span>
          </div>
          {u.cwd ? <div className="truncate font-mono text-muted-foreground" title={u.cwd}>{u.cwd}</div> : null}
          <div className="truncate font-mono text-muted-foreground" title={u.args}>{u.args}</div>
          {u.launcher ? <div className="truncate text-muted-foreground" title={u.launcher}>{t("add.startedBy", { cmd: u.launcher })}</div> : null}
        </div>
      ))}
    </div>
  );
}

function ScanResult({ r }: { r: HostScanResponse }) {
  const s = r.scan;
  const roots = s.roots ?? [];
  const unmanaged = s.unmanaged ?? [];
  const now = Date.now() / 1000;
  return (
    <div className="flex flex-col gap-3 text-sm">
      <div>{t("add.machine", { host: s.hostname, os: s.os, arch: s.arch, cpus: s.cpus, mem: s.mem_total_gb.toFixed(0), free: s.mem_avail_gb.toFixed(0) })}</div>
      {!s.tmux ? <div className="text-xs text-amber-700 dark:text-amber-300">{t("add.noTmux")}</div> : null}
      <div>
        <div className="mb-1 text-xs font-semibold">{t("add.workspaces")}</div>
        {roots.length ? roots.map((root) => (
          <div key={root.root} className="text-xs">
            <span className="font-mono">{root.root}</span>
            {(root.projects ?? []).map((p) => (
              <div key={p.name} className="pl-4 text-muted-foreground">{t("add.project", { name: p.name, s: p.sessions, r: p.running, res: p.results })}</div>
            ))}
          </div>
        )) : <div className="text-xs text-muted-foreground">{t("add.none")}</div>}
      </div>
      <div>
        <div className="mb-1 text-xs font-semibold">{t("add.unmanaged")} ({unmanaged.length})</div>
        {unmanaged.length ? <UnmanagedList items={unmanaged} now={now} /> : <div className="text-xs text-muted-foreground">{t("add.none")}</div>}
      </div>
      {s.others_busy ? <div className="text-xs text-muted-foreground">{t("add.othersBusy", { n: s.others_busy })}</div> : null}
    </div>
  );
}

/** A dialog that scans a server by its address and adds it to the hosts. */
export default function AddHostDialog({ onClose }: { onClose: () => void }) {
  const [form, setForm] = useState<ScanForm>({ address: "", name: "", root: "", akitaDir: "" });
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState<"scan" | "add" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<HostScanResponse | null>(null);
  const [entry, setEntry] = useState<EntryForm | null>(null);
  const [added, setAdded] = useState<string | null>(null);

  const scan = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.address.trim()) return;
    setBusy("scan"); setError(null); setResult(null); setEntry(null);
    try {
      const r = await scanHost(form);
      setResult(r); setEntry(entryForm(r.host));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const add = async () => {
    if (!result || !entry) return;
    setBusy("add"); setError(null);
    try {
      const r = await addHost(entryFrom(result.host, entry));
      setAdded(r.added);
      refreshNow();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  const set = (k: keyof ScanForm) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value });
  const setE = (k: keyof EntryForm) => (e: { target: { value: string } }) => entry && setEntry({ ...entry, [k]: e.target.value });
  const taken = !!result && entry?.name === result.host.name && result.taken;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-auto bg-black/40 p-4" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={t("add.title")}
        className="mt-8 flex w-full max-w-2xl flex-col gap-4 rounded-lg border bg-background p-4 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold">{t("add.title")}</h2>
          <button type="button" className="text-muted-foreground hover:text-foreground" onClick={onClose} aria-label={t("add.close")}><X className="h-4 w-4" /></button>
        </div>

        {added ? (
          <>
            <div className="text-sm">{t("add.added", { name: added })}</div>
            <div className="flex justify-end"><Button onClick={onClose}>{t("add.close")}</Button></div>
          </>
        ) : (
          <>
            <form className="flex flex-col gap-3" onSubmit={scan}>
              <Field label={t("add.address")} hint={t("add.addressHint")}>
                <div className="flex gap-2">
                  <input className={inputClass} value={form.address} onChange={set("address")} placeholder="me@server1.example.edu" autoFocus />
                  <Button type="submit" size="sm" disabled={!!busy || !form.address.trim()}>
                    {busy === "scan" ? <Loader2 className="animate-spin" /> : null}{t("add.scan")}
                  </Button>
                </div>
              </Field>
              <button type="button" className="self-start text-xs text-primary hover:underline" onClick={() => setMore(!more)}>{t("add.more")}</button>
              {more ? (
                <div className="grid gap-3 sm:grid-cols-3">
                  <Field label={t("add.name")} hint={t("add.nameHint")}><input className={inputClass} value={form.name} onChange={set("name")} /></Field>
                  <Field label={t("add.root")} hint={t("add.rootHint")}><input className={inputClass} value={form.root} onChange={set("root")} /></Field>
                  <Field label={t("add.akitaDir")} hint={t("add.akitaDirHint")}><input className={inputClass} value={form.akitaDir} onChange={set("akitaDir")} /></Field>
                </div>
              ) : null}
            </form>

            {busy === "scan" ? <div className="text-xs text-muted-foreground">{t("add.scanning")}</div> : null}
            {error ? <div className="whitespace-pre-wrap break-all text-xs text-red-600 dark:text-red-400">{error}</div> : null}

            {result && entry ? (
              <>
                <ScanResult r={result} />
                <div className="flex flex-col gap-3 border-t pt-3">
                  <div className="text-xs font-semibold">{t("add.entry")}</div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field label={t("add.name")}><input className={inputClass} value={entry.name} onChange={setE("name")} /></Field>
                    <Field label={t("add.root")}><input className={inputClass} value={entry.root} onChange={setE("root")} /></Field>
                    <Field label={t("add.slots")} hint={t("add.slotsHint")}><input className={inputClass} inputMode="numeric" value={entry.slots} onChange={setE("slots")} /></Field>
                    <Field label={t("add.reserve")}><input className={inputClass} inputMode="decimal" value={entry.reserve} onChange={setE("reserve")} /></Field>
                  </div>
                  {taken ? <div className="text-xs text-red-600 dark:text-red-400">{t("add.taken")}</div> : null}
                  <div className="text-xs text-muted-foreground">{t("add.to", { config: result.config })}</div>
                  <div className="flex justify-end gap-2">
                    <Button variant="outline" onClick={onClose}>{t("add.cancel")}</Button>
                    <Button onClick={add} disabled={!!busy || taken || !entry.name.trim()}>
                      {busy === "add" ? <Loader2 className="animate-spin" /> : null}{t("add.add")}
                    </Button>
                  </div>
                </div>
              </>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}
