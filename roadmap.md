# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model with fully serializable State.

## Completed Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2
### M5: Redesign Messages as Plain Structs ✅ — Budget: 8, Used: 6

Replaced `sim.Msg` interface with concrete `Msg` struct. All 31 message types converted to payload structs. PR #14 merged.

### M6: Port All First-Party Components ✅

#### M6.1–M6.4: Ported all 16 tick-driven components structurally ✅ — Budget: 16, Used: 8

### M7: Move Mutable Runtime Data into State Structs — IN PROGRESS

#### M7.1: Simple components (no queueing/cache deps) ✅ — Budget: 6, Used: 4
Populated State for: mmuCache, mmu, gmmu, addresstranslator, datamover, endpoint.

#### M7.2: Components with queueing deps ✅ — Budget: 6, Used: 4
Added queueing snapshot functions (SnapshotPipeline/RestorePipeline/SnapshotBuffer/RestoreBuffer). Populated State for: simplebankedmemory, switches, TLB. PR #20 merged.

#### M7.3: DRAM State Population ✅ — Budget: 6, Used: 2
Populated State for DRAM with full pointer-graph flattening (527-line state.go). PR #21 merged.

#### M7.4: Cache State Population (writearound, writeevict, writethrough) ✅ — Budget: 6, Used: 2
Shared Directory/MSHR serialization helpers in v5/mem/cache/state_helpers.go. State population for 3 near-identical cache variants. PR #22 merged.

#### M7.5: Writeback Cache State Population — DEFERRED
Analysis complete (Diana's report). ~565 lines estimated. **Deferred pending Msg redesign decision** — building this with MsgRef would require rewriting after the redesign.

## Current Decision Point: Msg Redesign (Human Issue #93)

Human proposed making `Msg` an interface with `Src()`, `Dst()`, `Serialize()`, `Deserialize()` methods, with each package implementing concrete serializable message types. This would:
- Eliminate the `Payload any` field and the MsgRef/Payload-loss bug
- Each package owns its message types (mem.ReadReq, vm.TranslationReq, etc.)
- Make state serialization natural (store concrete types, no MsgRef conversion)
- But reverses M5's Msg-as-struct decision and touches ~120+ files

**Status**: Discussing with human. Research workers analyzing concrete design + risks.

### M8: Msg-as-Interface Redesign — PLANNING
Pending design finalization. Key open questions:
- Exact interface definition (Serialize/Deserialize on interface vs external)
- FlitPayload inner Msg serialization
- Info interface{} field on ReadReq/WriteReq
- Migration strategy (incremental vs big-bang)

After M8, M7.5 (writeback state) can be completed with the new message types.

### M9: Complete Remaining State Population — AFTER M8
- Writeback cache state (was M7.5)
- Update M7.1-M7.4 state.go files to use concrete message types instead of MsgRef
- Fix Payload loss bug (should be automatically resolved by concrete types)

### M10: Comp Wrapper Elimination — AFTER M9
Human issue #61: eliminate the `Comp` wrapper struct. With fully serializable State and concrete message types, reassess whether Comp is still needed.

## Known Bugs
- Payload loss in ALL msgRef implementations → nil Payload after save/load → panics (will be fixed by M8)
- 6 identical msgRef definitions across packages (will be eliminated by M8)

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components structurally ported (State populated for 14/15; remaining: writeback)
5. **CI Passes** — All checks green on main

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20).
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's/Diana's detailed analysis before milestones prevents scope misestimation.
- Multi-worker parallel approach is the winning pattern for mechanical refactoring.
- M6 sub-milestones consistently completed in 2 cycles each (budgeted 4).
- Research for M7 revealed that the queueing/cache serialization problem is deeper than expected — need phased approach.
- The `*sim.Msg` pointer problem is universal across all components — the idealmemcontroller decomposition pattern is the proven solution.
- queueing snapshot functions (Approach A) proved pragmatic — standalone functions avoiding interface changes + no mock updates.
- M7.1–M7.4 each completed well under budget — consistent track record. Total: 12 cycles used vs 24 budgeted.
- Total project: ~41 implementation cycles across 78 orchestrator cycles. Overhead from planning/verification is worthwhile (catches real bugs like the Payload loss).
- **NEW**: When a core design issue surfaces (like the Payload problem), it's better to address it before building more on the broken foundation. M7.5 deferred to avoid rework.
