// Small keyed caches for /api/v1/campaign and /api/v1/run ("host|project|name").
import { useSyncExternalStore } from "react";
import { keyParts, type CampaignCells, type Run } from "./types.ts";

export interface Entry<T> {
  loading: boolean;
  error?: string;
  data?: T;
  at: number; // ms of the last finished fetch
  /** Finished cells the status list showed when this fetch started. */
  seen?: number | null;
}

class KeyedCache<T> {
  entries: Record<string, Entry<T>> = {};
  private listeners = new Set<() => void>();
  private path: string;
  constructor(path: string) { this.path = path; }

  subscribe = (l: () => void) => { this.listeners.add(l); return () => { this.listeners.delete(l); }; };
  snapshot = () => this.entries;

  private set(key: string, e: Entry<T>) {
    this.entries = { ...this.entries, [key]: e };
    this.listeners.forEach((l) => l());
  }

  async load(key: string, refresh = false, seen: number | null = null): Promise<void> {
    const prev = this.entries[key];
    if (prev?.loading) return;
    this.set(key, { ...(prev ?? { at: 0 }), loading: true, seen });
    const { host, project, name } = keyParts(key);
    const q = new URLSearchParams({ host, project, name });
    if (refresh) q.set("refresh", "1");
    let next: Entry<T>;
    try {
      const r = await fetch(`${this.path}?${q}`, { cache: "no-store" });
      const d = await r.json();
      if (!r.ok || d?.error) next = { loading: false, error: d?.error || `HTTP ${r.status}`, data: prev?.data, at: Date.now(), seen };
      else next = { loading: false, data: d as T, at: Date.now(), seen };
    } catch (e) {
      next = { loading: false, error: e instanceof Error ? e.message : String(e), data: prev?.data, at: Date.now(), seen };
    }
    this.set(key, next);
  }
}

export const campaignCache = new KeyedCache<CampaignCells>("/api/v1/campaign");

export interface RunDetail {
  name?: string;
  project?: string;
  meta?: Record<string, unknown> | null;
  config?: { flags?: Record<string, string>; command_line?: string[]; files?: string[] } | null;
  metrics?: {
    count: number;
    fingerprint: string;
    agg: Record<string, { sum: number; n: number; min: number; max: number }>;
    nonfinite?: Record<string, number>;
  } | null;
  summary?: string | null;
  error?: string;
}
export const runCache = new KeyedCache<RunDetail>("/api/v1/run");

export function useCache<T>(c: KeyedCache<T>): Record<string, Entry<T>> {
  return useSyncExternalStore(c.subscribe, c.snapshot, c.snapshot);
}

/**
 * First load, retry after errors (after a minute), and reload with refresh=1
 * when the status list's count of finished cells changed since the last load
 * (at most every 15 s), or every minute for an unfinished campaign the status
 * list has no progress for.
 */
export function maybeLoadCampaigns(sel: string[], runs: Run[]): void {
  for (const key of sel) {
    const { host, project, name } = keyParts(key);
    const r = runs.find((x) => x.host === host && x.project === project && x.name === name);
    const finished = r?.progress ? r.progress.finished : null;
    const st = campaignCache.entries[key];
    if (!st) { void campaignCache.load(key, false, finished); continue; }
    if (st.loading) continue;
    const age = Date.now() - st.at;
    if (!st.data) { if (age > 60000) void campaignCache.load(key, false, finished); continue; }
    if (finished != null ? finished !== st.seen && age > 15000 : !cellsFinished(st.data) && age > 60000) {
      void campaignCache.load(key, true, finished);
    }
  }
}

/** No cell of the campaign is running or missing. */
export function cellsFinished(d: CampaignCells): boolean {
  return !(d.cells ?? []).some((c) => c.status === "running" || c.status === "missing");
}
