# Tracing Instrumentation Plan

This plan captures the tracing/instrumentation work for the Akita simulator: the
component-task lifecycle (`req_in`/`req_out`), milestones (blocking-reason
intervals), tags (categorical labels), and the new per-phase subtasks
(port-buffer queueing, network transfer).

It folds together (a) the new task decomposition we are building for the data
path and (b) the correctness issues found in the cross-component instrumentation
audit.

## Background — the model

A traced round trip is a `req_out` task (opened by the sender) with child
subtasks for each phase of the request's life:

1. network transfer of the request
2. **incoming-buffer queueing of the request** — *done, see Step 0*
3. processing (`req_in`) — recorded today
4. network transfer of the response
5. incoming-buffer queueing of the response

Within a processing (`req_in`) task, **milestones** mark the moment each blocking
condition resolves; the interval from the previous milestone (or task start) to
a milestone is the time the task was blocked on that reason. Milestone kinds:
`hardware_resource`, `network_busy`, `network_transfer`, `queue`, `data`,
`dependency`, `translation`, `subtask`, `other`. The `DBTracer` dedups milestones
by `(Kind, What)` keeping the first, and drops same-timestamp duplicates — so
retried emissions and zero-duration stalls collapse automatically.

Reference conventions: the TLB records rich milestones + tags; the address
translator and DRAM record milestones.

---

## Step 0 — Incoming-buffer queueing subtask (#2 + #5) — DONE

Reusable port hook that opens an `incoming_queue` task on `HookPosPortMsgRecvd`
and closes it on `HookPosPortMsgRetrieveIncoming`, parented to the `req_out`
task (`msg.ID`, or `RspTo` for responses). Emitted on the port's owning
component so it flows to that component's tracers; no-op when untraced.

- `tracing/incomingqueuetracer.go` — `incomingQueueHook` + `CollectIncomingQueueTrace`.
- `simulation/simulation.go` — wired into `RegisterPort` (global, mirrors how
  `RegisterComponent` attaches `CollectTrace`).
- Tests: `tracing/incomingqueuetracer_test.go`.

**Verified** on a traced `virtualmem` run: 440 `incoming_queue` tasks, clean
durations, 95% parented to `req_out`, exact retrieval-time alignment with
`req_in` at non-pipelined components (the TLB pipeline gap is correct). Captures
both the request side (#2) and, for free, the response side (#5).

---

## Step 1 — ROB processing-phase milestones + tags — DONE

**Objective.** Give the ROB the milestone/tag coverage every other data-path
component should have. This is the original goal that started this work.

The `req_in` task now records, in order:

| Milestone | Kind | Where | `What` | Marks |
|---|---|---|---|---|
| buffer slot acquired | `hardware_resource` | `topDown` | `<rob>.buffer` | waited for a free ROB entry |
| shadow req sent | `network_busy` (Bottom) | `topDown` | Bottom port | waited to send downstream |
| bottom response arrived | `data` (read) / `subtask` (write) | `parseBottom` | Bottom port | waited on the bottom unit |
| reached head of reorder buffer | `dependency` | `bottomUp` | `<rob>.reorder` | waited behind older transactions |
| response sent | `network_busy` (Top) | `bottomUp` | Top port | waited to send to top |

Plus a `read`/`write` tag, emitted once when the `req_in` opens (keyed off the
request's concrete type).

- `mem/rob/middleware.go` — five `AddMilestone` calls + the tag, with two small
  helpers (`reqInTaskID`, `tagReadWrite`). All milestones address the same
  `req_in` task (the top request's receiver-side ID), resolved via
  `MsgIDAtReceiver` of a reconstructed top request so `parseBottom`/`bottomUp`
  reach the live task before `TraceReqComplete` forgets it.
- The `dependency` milestone is emitted in `bottomUp` **after the `HasRsp`
  check, before `CanSend`** — no per-transaction flag: the `DBTracer`
  `(Kind, What)` dedup keeps the first emission across the `CanSend` retry, and
  the same-timestamp rule both lets the more-meaningful `dependency` win a
  same-tick tie over the response-sent milestone and collapses zero-length
  head-of-line waits.
- Tests: `tracing/incomingqueuetracer_test.go` unchanged; new
  `mem/rob/milestone_test.go` drives read and write round trips through a
  recording tracer and asserts the exact ordered set of kinds, the `What`
  labels, the shared non-zero task ID, the `read`/`write` tags, and that
  `dependency` precedes the response-sent milestone.

**Verified** by the new unit tests (`go test ./mem/rob/...`, golangci-lint
clean). Remaining integration check: a traced `virtualmem`/MGPUSim run, querying
the `milestone` table for the five ROB kinds and confirming `dependency`
intervals are ~zero with no reorder penalty and positive when there is one.

**Severity/effort.** Feature; small. Depended on nothing.

---

## Step 2 — Fix the address-translator stale-pointer finalize bug

**Objective.** Stop orphaning TLB child tasks (empirically 11 per short run).

**Scope.** `mem/vm/addresstranslator/respondpipelinemw.go`, `comp.go`.

**Problem.** `removeTransaction` (`comp.go:180`, an `append`-shift) runs at
`respondpipelinemw.go:97` **before** `traceTranslationComplete` reads
`trans.TranslationReqID` through the now-stale pointer (`:100`/`:129`). When the
completing transaction is not last in the queue, `TraceReqFinalize` ends the
**wrong** translation `req_out`; the correct one never ends, never gets written,
and its TLB `req_in`/`incoming_queue` children are orphaned. Tracing-only — the
functional path already uses local copies.

**Approach.** Capture `TranslationReqID/Src/Dst` (or build `fakeTransReq`) into
locals **before** `removeTransaction`, or reorder so the trace read precedes the
removal.

**Verification.** Re-run the orphan query — `req_in`@TLB with a missing parent
should drop to 0 (modulo any genuinely-untraced paths).

**Severity/effort.** SEV-1 (normal-run corruption), confirmed; small.

---

## Step 3 — Systemic fix: stop `handleReset` leaking in-flight tasks

**Objective.** Eliminate the most widespread defect: every stateful component
clears its in-flight container on Reset without ending open `req_in`/`req_out`
(and DRAM "sub-trans") tasks or calling `ForgetMsgIDAtReceiver` — producing
started-never-ended tasks and slow registry growth whenever a Reset races
traffic. (Drain paths are already clean — they let work finish.)

**Scope (11 components).** `handleReset` in:
`mem/rob/middleware.go:393`, `mem/vm/tlb/ctrlmiddleware.go:228`,
`mem/vm/addresstranslator/ctrlmiddleware.go`, `mem/cache/writeback/ctrlmiddleware.go:237`,
`mem/cache/writethroughcache/ctrlmiddleware.go:140`, `mem/datamover/ctrlmiddleware.go:124`,
`mem/vm/mmu/ctrlmiddleware.go:124`, `mem/vm/gmmu/ctrlmiddleware.go`,
`mem/dram/ctrlmiddleware.go:125`, `mem/idealmemcontroller/ctrlmiddleware.go:123`,
`mem/simplebankedmemory/ctrlmiddleware.go:128`.

**Approach.** Before clearing state, iterate the in-flight work and
`EndTask` + `ForgetMsgIDAtReceiver` each open task (mirroring the normal
completion path). Prefer a small shared helper so all 11 sites converge on one
correct pattern rather than 11 hand-rolled versions.

**Verification.** A control-contract-style test that issues `Reset` with
in-flight traffic and asserts the `DBTracer` has no unended tasks and an empty
receiver registry afterward.

**Severity/effort.** SEV-2 (rare trigger) but highest leverage; medium.

---

## Step 4 — Per-component lifecycle bug fixes (SEV-1)

These corrupt or miss data on **normal** runs, independent of reset.

1. **mmuCache — no data-path tracing.** `mmucachemiddleware.go` opens no
   `req_in` and initiates no `req_out` for the downstream `TranslationReq`; its
   only tracing is control-path milestones (`ctrlmiddleware.go:115,133,153,184,277`)
   that attach to **phantom tasks** (`MsgIDAtReceiver` with no `TraceReqReceive`).
   → Add `TraceReqReceive`/`TraceReqComplete` for Top requests and
   `TraceReqInitiate`/`TraceReqFinalize` for the Bottom fetch; only then are the
   control milestones meaningful.
2. **gmmu — three remote-path defects** (`walkmw.go`, `respondmw.go`):
   downstream `req_out` never initiated (`walkmw.go:176`); a spurious `req_in`
   opened **on the response** (`respondmw.go:51`); the original top `req_in`
   leaked on remote completion (`respondmw.go:60-90`). → Initiate/finalize the
   downstream req, drop the response-side `TraceReqReceive`, complete the top
   `req_in` when the remote response returns.
3. **datamover — wrong-key `req_in` close.** Opens on `DataMoveRequest.ID`
   (`ctrlparsemw.go:97`), closes on a fresh `DataMoveResponse.ID` (`:148`), so
   the `req_in` never ends and leaks a registry entry every transaction. → Close
   using a reconstruction carrying the request ID (as the ROB does via
   `topReqTraceMsg`).
4. **endpoint — `msg_e2e` dead code.** `tryDeliver` only calls with
   `isEnd=true`; the `StartTask` branch (`incomingmw.go:210-233`) is unreachable,
   so `msg_e2e` tasks are never recorded. → Either wire up the start or remove
   the vestigial `msg_e2e` path. (`flit_e2e` is correct.)

**Verification.** Static + a small traced scenario per component (e.g. exercise
mmuCache/gmmu via a translation hierarchy that instantiates them) and confirm
balanced, non-orphaned tasks.

**Severity/effort.** SEV-1; small–medium each.

---

## Step 5 — Milestone & tag coverage rollout

**Objective.** Extend the milestone convention to the data-path components that
record none, so blocked-time is attributable end-to-end.

**Scope / high-value milestones.**
- **Caches** (writeback, writethroughcache): `hardware_resource` (MSHR full,
  write-buffer/intake admission), `network_busy` (Top/Bottom `!CanSend`),
  `data`/`dependency` (waiting on the downstream fetch/eviction response),
  `queue` (bank/dir buffer waits).
- **simplebankedmemory:** `hardware_resource` for bank-busy / pipeline-cannot-accept.
- **DRAM:** `queue` for command/sub-trans-queue full; a milestone for the
  refresh stall (currently invisible).
- **gmmu/mmu:** the remote-fetch wait and walk-latency resolution.
- **ROB:** delivered by Step 1.

**Tag gaps (small).** writethrough write-through-policy write-hit untagged
(`writepolicy.go:209`); writeback `write-mshr-hit` emitted unconditionally
(`directorystage.go:219`).

**Severity/effort.** SEV-3 coverage; medium, incremental per component.

---

## Step 6 — Network-transfer subtask (#1 / #4)

**Objective.** Record the request/response network-transfer phases as their own
subtasks, completing the round-trip decomposition.

**Scope.** New cross-port tracer (the transfer spans the source's outgoing port
and the destination's incoming port — `HookPosPortMsgRetrieveOutgoing` →
`HookPosPortMsgRecvd`, correlated by msg ID), parented to the `req_out` task.
For `directconnection` the transfer is ~0 latency; the meaningful cost appears
under the real NoC (PCIe/mesh).

**Approach options.** (a) a tracer attached to both endpoint ports correlating
by msg ID — generalizes to multi-hop NoC since the original msg ID reappears
only at true source/destination; or (b) connection-level hooks. Decide scope
(all component ports vs opt-in) given the per-message volume.

**Severity/effort.** Feature; medium–large (the one architectural piece).

---

## Suggested order

1. **Step 1** (ROB milestones) — **DONE**; the original goal, self-contained.
2. **Step 2** (AT stale-pointer) — *next*; quick, confirmed correctness win;
   improves the quality of every translation trace including the ROB's.
3. **Step 3** (reset-leak helper) — highest leverage; one shared fix for 11
   components.
4. **Step 4** (per-component SEV-1) — normal-run correctness.
5. **Step 5** (milestone/tag rollout) — coverage, incremental.
6. **Step 6** (network-transfer subtask) — the remaining architectural phase.

## Confidence notes

- Step 0 (#2) and Step 2 (AT bug) are **empirically confirmed** on a traced
  `virtualmem` run.
- The reset-leak *pattern* is confirmed where exercised; the mmuCache/gmmu/
  datamover bugs (Step 4) are **high-confidence static findings** —
  `virtualmem`'s hierarchy does not instantiate those components, so confirm
  with a scenario that does before/after fixing.
