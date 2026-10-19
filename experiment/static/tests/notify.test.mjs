// Notifications on session state transitions (the old page's notifyTransitions).
import assert from "node:assert/strict";
import { test } from "node:test";
import { transitions } from "../src/lib/notify.ts";
import { setLocale } from "../src/lib/i18n.ts";

const S = (id, state, extra = {}) => ({ id, state, archived: false, group: null, exit_code: null, health: {}, ...extra });

test("end, stall and group-finished notes", () => {
  setLocale("zh", false);
  let { notes, memo } = transitions(null, [S("h/p/a", "running", { group: "g" }), S("h/p/b", "running", { group: "g" }), S("h/p/c", "running")]);
  assert.equal(notes.length, 0);
  ({ notes, memo } = transitions(memo, [S("h/p/a", "ok", { group: "g", exit_code: 0 }), S("h/p/b", "running", { group: "g" }),
    S("h/p/c", "stalled", { health: { hang_reason: "WG 15 分钟不增加" } })]));
  assert.deepEqual(notes.map((n) => n.title), ["实验结束", "实验疑似卡住"]);
  assert.match(notes[0].body, /h\/p\/a：完成（exit 0）/);
  assert.match(notes[1].body, /WG 15 分钟不增加/);
  ({ notes, memo } = transitions(memo, [S("h/p/a", "ok", { group: "g" }), S("h/p/b", "failed", { group: "g", exit_code: 1 }),
    S("h/p/c", "stalled")]));
  assert.deepEqual(notes.map((n) => n.title), ["实验结束", "一组实验全部结束"]);
  assert.equal(notes[1].body, "组 g：完成 1 个，失败或中断 1 个");
  // a stalled session that dies counts as ended; archived sessions are ignored
  ({ notes } = transitions(memo, [S("h/p/c", "died"), S("h/p/a", "ok", { archived: true })]));
  assert.deepEqual(notes.map((n) => n.title), ["实验结束"]);
  setLocale("en", false);
  ({ notes } = transitions(transitions(null, [S("h/p/a", "running")]).memo, [S("h/p/a", "failed", { exit_code: 2 })]));
  assert.deepEqual(notes, [{ title: "Session ended", body: "h/p/a: Failed (exit 2)" }]);
});
