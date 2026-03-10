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

#### M7.1: Simple components ✅ — Budget: 6, Used: 2
Populated State for 6 components: mmuCache, mmu, gmmu, addresstranslator, datamover, endpoint. All validated by Apollo. PR #19 merged.

## Upcoming Milestones

### M7 continued: 8 components still have empty State structs

Remaining components:
1. **simplebankedmemory** — queueing.Pipeline/Buffer per bank
2. **switches** — queueing.Pipeline/Buffer per port complex, routing table, arbiter
3. **TLB** — queueing.Pipeline/Buffer, mshr, sets, *sim.Msg
4. **DRAM** — deep internal packages (cmdq, trans, org, signal)
5. **writearound** — cache.Directory, cache.MSHR, queueing.Buffer, stage structs, transactions
6. **writeback** — most complex cache variant
7. **writeevict** — similar to writearound
8. **writethrough** — similar to writearound

**Key blocker**: `queueing.Pipeline` and `queueing.Buffer` are interfaces. Their internals (stage contents, cycle counters, buffer elements) are not accessible for serialization. Need either:
- Add snapshot/restore methods to existing queueing interfaces
- Create `queueingv5` package with concrete serializable types (per migration.md vision)
- Manual extraction approach per component

#### M7.2: TBD — Planning in progress (cycle 62)
Diana and Iris analyzing approaches. Expected scope: queueing serialization + simpler components.

#### M7.3: TBD — Cache components
Most complex group, deferred until queueing approach proven.

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components structurally ported
5. **CI Passes** — All checks green on main
6. **M7.1 State Population** — 6 simple components have serializable State

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20).
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's/Diana's detailed analysis before milestones prevents scope misestimation.
- Multi-worker parallel approach is the winning pattern for mechanical refactoring.
- M6 sub-milestones consistently completed in 2 cycles each (budgeted 4).
- M7.1 completed in 2 cycles (budgeted 6) — team is efficient at this pattern now.
- Total project so far: ~29 implementation cycles + ~12 planning/verification cycles across 62 orchestrator cycles.
- Research for M7 revealed that the queueing/cache serialization problem is deeper than expected — need phased approach.
- The `*sim.Msg` pointer problem is universal — the idealmemcontroller decomposition pattern is proven.
