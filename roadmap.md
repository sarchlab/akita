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

## Upcoming Milestones

### M7: Move Mutable Runtime Data into State Structs — IN PROGRESS

**Problem**: 13/15 ported components have empty `State struct{}`. Mutable data lives on Comp wrapper, not in State. Components aren't truly serializable.

**Research findings** (from Diana/Iris analysis):
- Category 1 (~30 fields): Plain primitives, easy to move to State/Spec
- Category 2: queueing.Buffer/Pipeline (interface-based, NOT serializable), cache.Directory/MSHR (pointer-heavy), transaction slices with `*sim.Msg`
- Category 3: sim.Port, mappers, page tables — runtime handles, stay on Comp (reconstructed, not serialized)
- Universal blocker: `*sim.Msg` in all transaction types — must decompose into ID+Src+payload fields (per idealmemcontroller pattern)
- `queueingv5` package does NOT exist yet — migration.md describes intended future design

**Approach**: Phase the work by component complexity:

#### M7.1: Simple components (no queueing/cache deps) — Budget: 6
Move mutable data into State for:
- mem/vm/mmuCache, mem/vm/mmu, mem/vm/gmmu, mem/vm/addresstranslator
- mem/datamover, noc/networking/switching/endpoint

Key patterns: decompose `*sim.Msg` into plain fields, make internal Set types serializable, replace container/list with plain slices, move immutable config from Comp to Spec.

#### M7.2: Components with queueing deps — Budget: TBD
TLB, simplebankedmemory, switches, DRAM — need serializable queueing implementation first.

#### M7.3: Cache components — Budget: TBD
writearound, writeback, writeevict, writethrough — most complex, need serializable Directory + MSHR + stage state.

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components structurally ported (State structs mostly empty)
5. **CI Passes** — All checks green on main

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20).
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's/Diana's detailed analysis before milestones prevents scope misestimation.
- Multi-worker parallel approach is the winning pattern for mechanical refactoring.
- M6 sub-milestones consistently completed in 2 cycles each (budgeted 4).
- Total project so far: ~27 implementation cycles + ~10 planning/verification cycles = ~37 active across 56 orchestrator cycles.
- Research for M7 revealed that the queueing/cache serialization problem is deeper than expected — need phased approach.
- The `*sim.Msg` pointer problem is universal across all components — the idealmemcontroller decomposition pattern is the proven solution.
