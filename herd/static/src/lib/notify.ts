// Desktop notifications on session state transitions (as the old page):
// a session that stops running, one that becomes stalled, and a group whose
// members have all stopped running.
import { fmtDur } from "./format.ts";
import { hasMsg, t } from "./i18n.ts";
import type { Session } from "./types.ts";

/** Display name of a session state. */
export function stateLabel(state: string): string {
  const k = `state.${state}`;
  return hasMsg(k) ? t(k) : state;
}

export interface NotifyMemo {
  sessions: Record<string, Session>;
  groups: Record<string, { active: number; ok: number; bad: number }>;
}

export interface Note { title: string; body: string }

const active = (s: Session) => s.state === "running" || s.state === "stalled";

/** The notes for the step prev -> sessions, and the memo for the next step. */
export function transitions(prev: NotifyMemo | null, sessions: Session[]): { notes: Note[]; memo: NotifyMemo } {
  const cur: Record<string, Session> = {};
  for (const s of sessions) if (!s.archived) cur[s.id] = s;
  const notes: Note[] = [];
  if (prev) {
    for (const [key, s] of Object.entries(cur)) {
      const before = prev.sessions[key];
      if (!before) continue;
      if (active(before) && !active(s)) {
        notes.push({ title: t("note.ended"), body: `${key}${t("common.colon")}${stateLabel(s.state)}` + (s.exit_code != null ? t("note.exit", { code: s.exit_code }) : "") });
      } else if (before.state !== "stalled" && s.state === "stalled") {
        const h = s.health ?? {};
        notes.push({ title: t("note.stalled"), body: `${key}${t("common.colon")}` + (h.hang_reason || t("note.lowCpu", { cpu: (h.cpu_now || 0).toFixed(0), dur: fmtDur(h.low_for) })) });
      }
    }
  }
  const groups: NotifyMemo["groups"] = {};
  for (const s of Object.values(cur)) {
    if (!s.group) continue;
    const g = groups[s.group] || (groups[s.group] = { active: 0, ok: 0, bad: 0 });
    if (active(s)) g.active++;
    else if (s.state === "ok") g.ok++;
    else g.bad++;
  }
  if (prev) {
    for (const [name, g] of Object.entries(groups)) {
      const before = prev.groups[name];
      if (before && before.active > 0 && g.active === 0) {
        notes.push({ title: t("note.groupDone"), body: t("note.groupBody", { name, ok: g.ok }) + (g.bad ? t("note.groupBad", { n: g.bad }) : "") });
      }
    }
  }
  return { notes, memo: { sessions: cur, groups } };
}
