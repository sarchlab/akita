// Adding a server from the UI: scan it (POST /api/v1/hosts/scan), then add
// the edited entry to the user config (POST /api/v1/hosts). Both requests
// carry X-Herd-Request, which the server requires of requests that change
// something.
import type { HostConfig, HostScanResponse } from "./types.ts";

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Herd-Request": "1" },
    body: JSON.stringify(body),
  });
  const data = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((data as { error?: string }).error || `HTTP ${r.status}`);
  return data as T;
}

export interface ScanForm { address: string; name: string; root: string; akitaDir: string }

export function scanHost(f: ScanForm): Promise<HostScanResponse> {
  return postJSON("/api/v1/hosts/scan", {
    address: f.address.trim(), name: f.name.trim() || undefined, root: f.root.trim() || undefined,
    akita_dir: f.akitaDir.trim() || undefined,
  });
}

export function addHost(h: HostConfig): Promise<{ added: string; config: string }> {
  return postJSON("/api/v1/hosts", { host: h });
}

/** The editable fields of a proposed host entry. */
export interface EntryForm { name: string; root: string; slots: string; reserve: string }

export function entryForm(h: HostConfig): EntryForm {
  return {
    name: h.name, root: h.root ?? "~/exp",
    slots: h.queue?.slots != null ? String(h.queue.slots) : "",
    reserve: h.queue?.mem_reserve_gb != null ? String(h.queue.mem_reserve_gb) : "",
  };
}

/** The entry to add: the proposal with the edited fields; no queue when
 * slots is empty, no root when it is the default. */
export function entryFrom(proposed: HostConfig, f: EntryForm): HostConfig {
  const out: HostConfig = { name: f.name.trim(), type: proposed.type, address: proposed.address };
  const root = f.root.trim();
  if (root && root !== "~/exp") out.root = root;
  if (proposed.akita_dir) out.akita_dir = proposed.akita_dir;
  const slots = Number(f.slots), reserve = Number(f.reserve);
  if (f.slots.trim() && Number.isInteger(slots) && slots > 0) {
    out.queue = { ...(proposed.queue ?? {}), slots };
    if (f.reserve.trim() && reserve >= 0) out.queue.mem_reserve_gb = reserve;
    else delete out.queue.mem_reserve_gb;
  }
  return out;
}

/** The result name a launcher's command line mentions: its first argument
 * after the script that looks like a name (letters, digits, _ . + -, with
 * a "_"), or "". */
export function launcherRun(launcher?: string): string {
  const words = (launcher ?? "").split(/\s+/).filter(Boolean);
  const script = words.findIndex((w) => /\.(sh|py|bash)$/.test(w));
  for (const w of words.slice(script + 1)) {
    if (!w.startsWith("-") && w.includes("_") && /^[A-Za-z0-9_.+-]+$/.test(w)) return w;
  }
  return "";
}
