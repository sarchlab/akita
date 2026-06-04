# Memory Agent Control Protocol

## Summary

This plan defines a uniform control protocol for every memory agent in Akita
(caches, TLBs, MMU caches, address translators, ROB, memory controllers,
DRAM, datamover) and lays out a phased migration to bring all current
implementations onto it.

A unified `mem.ControlReq`/`mem.ControlRsp` already exists in
`mem/protocol.go` but is partially adopted, inconsistently implemented, and
overloaded with single-purpose modifier flags. This plan tightens the verb
set to six well-defined operations, removes the overloading, and adds the
verbs that are currently missing from each component.

## Verbs

Six verbs. Four are universal — every memory agent must implement them.
Two are conditional — only agents that hold private cache-of-memory state
support them.

| Verb           | Universal? | Definition                                                                                                                                                                                                                                          | Ack timing                  |
| -------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------- |
| **Pause**      | ✓          | Stop accepting new traffic from Top/Bottom; stop scheduling internal work *now*. Queued and in-flight transactions are frozen in place — not discarded, not advanced. Pause is idempotent.                                                          | Sync, on receive.           |
| **Drain**      | ✓          | Stop accepting new traffic; let all in-flight transactions finish cleanly; end in the Paused state. The Rsp signals that the component is fully quiescent.                                                                                          | Async, on completion.       |
| **Enable**     | ✓          | Resume normal processing from Paused. Idempotent if already enabled.                                                                                                                                                                                | Sync, on receive.           |
| **Reset**      | ✓          | Hard reset to a freshly-built state: discard all in-flight transactions, clear internal queues, clear private caches (if any), drain Top/Bottom port queues. End state matches what `Build()` produced. Reset may be issued from any current state. | Sync, after state is wiped. |
| **Invalidate** | conditional | Drop entries from the agent's private state (cache lines, TLB entries, MMU-cache entries) without writeback. Component must be Paused or Drained first. May be filtered by `Addresses` and `PID`; empty filter = all.                              | Sync, after entries are dropped. |
| **Flush**      | conditional | Write back any dirty private state to the backing memory. Clean entries remain valid. Component must be Paused or Drained first. May be filtered by `Addresses` and `PID`; empty filter = all dirty data.                                          | Async, on writeback completion. |

### Why these and not others

- **No "Join."** The voice transcription suggested a "Join" verb; on inspection the closest semantic is Drain, which is already covered.
- **No `PauseAfter` / `DiscardInflight` / `InvalidateAfter` modifiers.** Each is a composition of the verbs above. The caller sequences them: `Drain → Flush → (stay paused)`, `Pause → Reset`, `Drain → Flush → Invalidate → Enable`. Cleaner protocol, easier to test, fewer state-machine corners per component.
- **Reset is distinct from Flush.** Today the writeback and writethrough caches both implement "Flush" as "writeback + wipe directory + clear queues." That is a Reset that happens to write back first. Splitting the two lets a caller take a non-destructive checkpoint of dirty data (Flush) without losing the warm cache, and lets a caller hard-reset (Reset) without paying writeback cost when they don't need it.
- **No "Restart."** TLB and mmuCache name their hard-reset handler "Restart." That is just Reset; rename for consistency.

## Protocol types

Final shape of `mem/protocol.go` once this work is done:

```go
type ControlCommand int

const (
    CmdPause      ControlCommand = iota
    CmdDrain
    CmdEnable
    CmdReset
    CmdInvalidate
    CmdFlush
)

type ControlReq struct {
    messaging.MsgMeta
    Command   ControlCommand
    Addresses []uint64 // Invalidate, Flush: filter by address (empty = all)
    PID       vm.PID   // Invalidate, Flush: filter by PID (zero = all)
}

type ControlRsp struct {
    messaging.MsgMeta
    Command ControlCommand
    Success bool
    Error   string // Empty on success. Set when the component does not
                   // support Command, or when the command is illegal in
                   // the current state.
}
```

Removed fields: `DiscardInflight`, `InvalidateAfter`, `PauseAfter`. These
were modifier flags that encoded compound operations; the new protocol
expects the caller to compose them from primitives.

## Conventions

These apply uniformly to every component.

1. **Control port.** Every memory agent exposes a port named `Control`. All `ControlReq`/`ControlRsp` messages — and only those — flow through it. Workload requests (reads, writes, translations, data-move requests) never use this port; they get their own ports (`Top`, `Bottom`, etc.). This applies uniformly: caches, TLBs, MMU, GMMU, MMU cache, address translator, ROB, DRAM, idealmemcontroller, simplebankedmemory, and datamover all gain a real `Control` port if they do not already have one.
2. **Datamover port rename.** Datamover currently uses `Control` as its workload-request port. That port is renamed (proposed: `Top`) and a new real `Control` port is added. This is a breaking change for callers; see Phase 2.
3. **Unsupported verbs.** A component that does not implement a given verb still responds. It sends `ControlRsp{Command: <verb>, Success: false, Error: "unsupported"}`. No panics on unknown verbs.
4. **Illegal-state verbs.** Invalidate and Flush require the component to be Paused or Drained. Issuing them while Enabled returns `Success: false, Error: "must be paused or drained"`. Reset is legal from any state.
5. **Rsp timing per verb.** Sync verbs (Pause, Enable, Reset, Invalidate) ack on receive — the Rsp goes out the same tick the request is accepted. Async verbs (Drain, Flush) ack when the operation completes; the request is accepted silently.
6. **State machine.** Each agent has a single `ControlState` enum: `{Enabled, Pausing, Paused, Draining, Flushing}`. Reset and Invalidate are operations within those states, not separate states. Pausing is the transient state between receiving Pause and finishing the current tick; most components can collapse Pausing → Paused inside the same tick.
7. **Idempotency.** Pause-when-Paused, Enable-when-Enabled, Drain-when-Paused all succeed without side effects.
8. **Reset is a hard signal.** Reset is processed unconditionally regardless of current state. If an async verb (Drain or Flush) is in flight when Reset arrives, the in-flight verb is dropped — its caller never gets a Rsp for it. Preventing concurrent Reset + async control is the sender's responsibility, not the component's.
9. **Tracing.** Every accepted ControlReq emits a milestone via `tracing.AddMilestone`. Completion of async verbs emits `tracing.TraceReqComplete`.

## Support matrix (target state)

|                              | Pause | Drain | Enable | Reset | Invalidate | Flush |
| ---------------------------- | ----- | ----- | ------ | ----- | ---------- | ----- |
| `cache/writeback`            | ✓     | ✓     | ✓      | ✓     | ✓          | ✓     |
| `cache/writethroughcache`    | ✓     | ✓     | ✓      | ✓     | ✓          | no-op |
| `vm/tlb`                     | ✓     | ✓     | ✓      | ✓     | ✓          | —     |
| `vm/mmuCache`                | ✓     | ✓     | ✓      | ✓     | ✓          | —     |
| `vm/mmu`                     | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `vm/gmmu`                    | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `vm/addresstranslator`       | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `rob`                        | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `idealmemcontroller`         | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `dram`                       | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `simplebankedmemory`         | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `datamover`                  | ✓     | ✓     | ✓      | ✓     | —          | —     |

`mshr` is intentionally absent from the matrix — it is a substructure of a
cache, not a standalone component, and has no control port of its own. Its
state is part of the enclosing cache's state and is wiped by the cache's
Reset (and dropped silently by the cache's Invalidate, per the policy in
"Resolved decisions" below).

`Flush` on the writethrough cache is "no-op" because the cache holds no
dirty data; the Rsp comes back immediately as `Success: true`. This keeps
the verb universally callable on any cache without the caller having to
branch on cache type.

## Migration phases

The work splits into four phases. Each phase ends with a clean working
tree (tests green, examples runnable) so we can stop or land a partial PR
at any phase boundary.

### Phase 1 — Protocol shape

Land the new protocol types in one go, before touching any component.

1. Rewrite `mem/protocol.go`:
   - Reorder `ControlCommand` constants to match the new enumeration order (Pause, Drain, Enable, Reset, Invalidate, Flush).
   - Remove `DiscardInflight`, `InvalidateAfter`, `PauseAfter` from `ControlReq`.
   - Add `Error string` to `ControlRsp`.
2. Update every current handler to compile against the new fields. Where a removed flag was being honored, leave a `// TODO(control-protocol): caller must now compose <X> + <Y>` and either preserve current behavior (default-case) or panic on the old flag combo — choose per call site so the build stays green.
3. Add `mem/control` (new subpackage) containing:
   - `ControlState` enum.
   - A `ControlContract` test harness: given a built component and the verb-support matrix entry for it, exercises every supported verb (happy path) and every unsupported verb (expects `Success: false, Error: "unsupported"`). One function: `TestControlContract(t, comp, matrix)`.
4. Single PR. CI green. No behavior changes yet beyond the field rename.

**Deliverables:** new `mem/protocol.go`, new `mem/control/contract.go`,
build green.

### Phase 2 — Universal verbs

Bring every component up to full support for Pause, Drain, Enable, Reset.
Order chosen so the easy components ship first and we accumulate test
infrastructure before touching the hard ones.

Component-by-component. Each step is a PR; each PR runs the
`ControlContract` test harness against the migrated component.

1. **`idealmemcontroller`** (smallest gap: needs Reset). Add `handleReset` to the existing ctrlMiddleware. Wire into contract test.
2. **`addresstranslator`** (needs Pause, Drain, Enable). Add the three handlers; the component's middleware already understands a paused-vs-running mode for the Reset case, so this is mostly bookkeeping.
3. **`vm/mmu`** (needs Control port + full middleware). Add a `Control` port in `builder.go`, add a `ctrlmiddleware.go` covering Pause/Drain/Enable/Reset. Drain waits for the in-flight walk queue to empty; Reset clears walk state but does not touch shared page-table resources (those are owned by the simulation, per the checkpointing model).
4. **`vm/gmmu`** (same shape as MMU). Add Control port + ctrlmiddleware. Drain waits for in-flight walks; Reset clears local walk state only.
5. **`dram`** (needs Control port + full middleware). Add a Control port in `builder.go`, add a `ctrlmiddleware.go` parallel to the existing per-bank tick middleware. Drain waits for `state.InflightTransactions == 0`.
6. **`simplebankedmemory`** (same as DRAM). Add Control port, add ctrlmiddleware.
7. **`rob`** (needs Pause, Drain; current "Flush" handler becomes Reset). Rename `handleFlush` → `handleReset` in `middleware.go`, change command match to `CmdReset`. Add `handlePause`, `handleDrain`. The existing `state.IsFlushing` becomes `state.ControlState == Paused`.
8. **`cache/writeback`** (needs Pause, Drain, separation of Flush and Reset). The current Flush implementation (`flusher.go`) is two operations stapled together: it walks the directory writing dirty blocks back (true Flush) and then wipes MSHR + directory (Reset side effect). Split:
   - `handleFlush` keeps the writeback walk but stops after `flushCompleted()`; it leaves directory state untouched (clean lines stay valid).
   - `handleReset` does the wipe (`cache.DirectoryReset`, MSHR clear, queue drains) without the writeback walk. MSHR is part of the cache, so this is where its state gets cleared.
   - Pause/Drain handlers added to the ctrl middleware. Drain calls `existInflightTransaction()` as its quiescence check.
9. **`cache/writethroughcache`** (same split as writeback). `hardResetCache()` in `controlstage.go` becomes the Reset handler — it already wipes MSHR alongside the directory. Flush is a no-op for this cache (no dirty data) — `handleFlush` immediately sends back `Success: true`. Pause/Drain added.
10. **`vm/tlb`** (rename Restart → Reset; ack semantics fix). Existing Pause/Drain/Enable have no Rsp; add them so the Rsp protocol is uniform. Rename `handleTLBRestart` → `handleReset`. Defer the Invalidate work to Phase 3 — leave the current Flush-with-filter as-is, marked TODO.
11. **`vm/mmuCache`** (same as TLB). Rename `handleMMUCacheRestart` → `handleReset`. Fix Rsp gaps.
12. **`datamover`** (port-rename + new ctrl middleware). Rename the existing `Control` port to `Top` everywhere it's used (`ctrlparsemw.go`, `builder.go`, `datamoving_test.go`, any examples/). Add a new `Control` port and a new `ctrlmiddleware.go` that handles Pause/Drain/Enable/Reset. Drain waits for `state.CurrentTransaction.Active == false`. This is the biggest blast radius of Phase 2 — do it last.

**Deliverables per component:** ctrl handlers, contract test passes,
existing component tests still green.

### Phase 3 — Conditional verbs

Add Invalidate (and the address-filtered Flush) where it applies.

1. **`vm/tlb`**: replace the current Flush-with-filter implementation with `handleInvalidate`. The handler reads `msg.Addresses` and `msg.PID` and removes matching entries from the TLB state. `handleFlush` returns `Success: false, Error: "unsupported"`.
2. **`vm/mmuCache`**: same as TLB.
3. **`cache/writeback`**: add `handleInvalidate` — drop matching blocks from the directory without writeback (warn or error if any matching block is dirty? — see Open Questions). Extend `handleFlush` to honor the address filter.
4. **`cache/writethroughcache`**: add `handleInvalidate` (clean drop). Flush remains a no-op-with-success.
5. Cross-component sequence tests in `mem/control`: Drain → Flush → Invalidate → Reset on a cache; Pause → Invalidate(filter) → Enable on a TLB.

### Phase 4 — Cleanup and examples

1. Remove the `// TODO(control-protocol)` markers planted in Phase 1; verify each call site now uses the composed sequence.
2. Update `examples/` that drive control sequences (find via `grep -rn "ControlReq" examples/`). The most common rewrite: `ControlReq{Cmd: CmdFlush, PauseAfter: true}` becomes `Drain` then `Flush`.
3. Update `mem/README.md` and add a short `mem/control/README.md` describing the protocol, the verbs, and the support matrix.
4. Run the acceptance suite (`mem/acceptance_test.py` / `mem/acceptancetests/`) end-to-end.

## Testing strategy

### Layer 1: contract harness (`mem/control/contract.go`)

A single function:

```go
func TestControlContract(
    t *testing.T,
    name string,
    build func() (comp modeling.AnyComponent, ctrl messaging.Port, teardown func()),
    matrix VerbSupport,
)
```

`VerbSupport` is a struct of six booleans (one per verb). The harness:

- For each supported verb: drives the verb through `ctrl`, asserts the right
  Rsp comes back with the right timing (sync vs async, per the verb's
  contract), asserts state observable from the contract harness (e.g.
  Drain → no in-flight after Rsp, Reset → component matches Build-state).
- For each unsupported verb: drives it, asserts `Success: false, Error: "unsupported"`.
- For Reset specifically: runs the verb from each reachable state (Enabled,
  Paused, Draining, Flushing) and asserts it always succeeds.
- For idempotency: drives the verb twice in a row, asserts both succeed.

Every component package adds one test that calls `TestControlContract` with
its build function and matrix entry. ~10 lines per component.

### Layer 2: per-component behavior tests

Existing per-component tests stay. They test behaviors specific to that
component (cache line tracking after Invalidate, dirty-line writeback during
Flush, TLB entry filtering by PID). The contract harness covers the protocol;
these tests cover the semantics.

For each component touched in Phase 2/3, add at minimum:

- **Drain quiescence**: enqueue N in-flight requests, send Drain, assert all
  N complete before Rsp comes back.
- **Pause freeze**: enqueue N requests, send Pause mid-stream, assert no
  further progress on top/bottom ports until Enable.
- **Reset from each state**: send Reset while Enabled, Paused, Draining,
  Flushing — assert all reach the same post-Reset state.

For caches and TLBs:

- **Filtered Invalidate**: populate, Invalidate with `{Addresses: [a], PID: p}`,
  assert only matching entries are gone.
- **Filtered Flush** (caches only): dirty a subset, Flush with filter,
  assert only matching dirty data was written back.

### Layer 3: cross-component sequence tests

A small integration test in `mem/control/sequence_test.go` builds a tiny
pipeline (CU port → cache → DRAM) and exercises full control sequences end
to end:

1. Run a small workload, Drain the cache, assert in-flight finished.
2. Flush the cache, assert DRAM received all dirty data.
3. Reset the cache, assert directory and MSHR are clean.
4. Enable, run the workload again, assert correctness.

This is the regression test for "we removed `PauseAfter` and `InvalidateAfter`
and everything still works." It is the load-bearing test for Phase 1.

### Layer 4: examples and acceptance

The `examples/` directory has runnable demos that exercise control today
(at least the ones using cache/tlb checkpointing). After Phase 4, every
example builds and runs. The `mem/acceptancetests/` suite is the
end-to-end gate for the migration.

## Resolved decisions

1. **Invalidate on dirty cache lines: drop silently.** Invalidate with no prior Flush discards dirty data without warning. The caller is responsible for issuing Flush first if the data must survive. This matches the current `cache.DirectoryReset` behavior.
2. **No Drain timeout.** Drain waits indefinitely for in-flight transactions to complete. Callers may assume Drain will always eventually finish.
3. **Reset is a hard signal.** Reset is processed unconditionally regardless of current state, including while Drain or Flush is in flight. The preempted async verb is dropped — its caller never receives a Rsp. Avoiding concurrent Reset + async control is the sender's responsibility.
4. **Every component has a `Control` port.** This includes components that today have none (DRAM, simplebankedmemory, MMU, GMMU) and the datamover (which renames its workload-overloaded `Control` port to `Top` and gains a real `Control` port). All `ControlReq`/`ControlRsp` flow through this port and nothing else.
5. **MSHR is out of scope as a standalone component, but its state belongs to the enclosing cache.** Cache Reset wipes MSHR; cache Invalidate of a line with an outstanding MSHR drops the MSHR entry silently. If MSHR ever becomes a standalone component (e.g., a shared MSHR pool), it inherits the universal-verb contract automatically.
