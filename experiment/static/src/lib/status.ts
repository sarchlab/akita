// Polls /api/v1/status every `interval` seconds. While the user is selecting
// text in the page the new snapshot is held back (the old page's behaviour),
// so a selection is not destroyed by a re-render.
import { useSyncExternalStore } from "react";
import { transitions, type NotifyMemo } from "./notify.ts";
import type { Status } from "./types.ts";

export interface StatusState {
  status: Status | null;
  error: string | null;
  paused: boolean; // a newer snapshot is waiting for the selection to end
}

let st: StatusState = { status: null, error: null, paused: false };
let pending: Status | null = null;
let memo: NotifyMemo | null = null;
let timer: number | null = null;
let started = false;
const listeners = new Set<() => void>();

function emit(next: Partial<StatusState>) {
  st = { ...st, ...next };
  listeners.forEach((l) => l());
}

function selecting(): boolean {
  const sel = window.getSelection();
  const main = document.querySelector("main");
  return !!(sel && !sel.isCollapsed && main && sel.anchorNode && main.contains(sel.anchorNode));
}

const BASE_TITLE = "AkitaExp";

/** Fill the lists a Go server sends as null when empty. */
export function normalizeStatus(s: Status): Status {
  const arr = <T,>(v: T[] | null | undefined): T[] => (Array.isArray(v) ? v : []);
  return {
    ...s,
    rules: s.rules ?? {},
    projects: s.projects ?? {},
    hosts: arr(s.hosts),
    sessions: arr(s.sessions).map((x) => ({
      ...x, group: x.group || null, tail: arr(x.tail), progress: arr(x.progress), sims: arr(x.sims), procs: arr(x.procs),
    })),
    runs: arr(s.runs).map((r) => ({ ...r, properties: arr(r.properties), failures: arr(r.failures) })),
  };
}

function apply(s: Status) {
  const { notes, memo: next } = transitions(memo, s.sessions || []);
  memo = next;
  for (const n of notes) {
    try {
      if ("Notification" in window && Notification.permission === "granted") new Notification(n.title, { body: n.body });
    } catch { /* ignore */ }
    document.title = `● ${n.title} — ${BASE_TITLE}`;
  }
  pending = null;
  emit({ status: s, error: null, paused: false });
}

async function poll() {
  let interval = 5;
  try {
    const r = await fetch("/api/v1/status", { cache: "no-store" });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    const s = normalizeStatus((await r.json()) as Status);
    interval = Math.max(2, Number(s.interval) || 5);
    if (s.updated) {
      if (st.status && selecting()) { pending = s; emit({ paused: true }); }
      else apply(s);
    }
  } catch (e) {
    emit({ error: e instanceof Error ? e.message : String(e) });
  }
  timer = window.setTimeout(poll, interval * 1000);
}

export function startPolling() {
  if (started) return;
  started = true;
  document.addEventListener("selectionchange", () => {
    if (pending && !selecting()) apply(pending);
  });
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) document.title = BASE_TITLE;
  });
  void poll();
}

export function refreshNow() {
  if (timer != null) window.clearTimeout(timer);
  void poll();
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}
const get = () => st;

export function useStatusState(): StatusState {
  return useSyncExternalStore(subscribe, get, get);
}
