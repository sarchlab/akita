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

#### M7.5: Writeback Cache State Population — NEXT
The only component with an empty State struct. Msg redesign is complete (M8), so no risk of rework. Can reuse state_helpers.go patterns from M7.4.

## M8: Msg-as-Interface Redesign ✅

Per human direction (#93), `sim.Msg` became an interface with concrete message types per package.

### M8.1: Foundation ✅ — Budget: 8, Used: 3
Renamed `Msg` → `GenericMsg`, added `Msg` interface. PR #23 merged.

### M8.2: Convert all payloads to concrete types + remove GenericMsg ✅ — Budget: 8, Used: 9
30 concrete message types, removed GenericMsg/MsgPayload/TryMsgPayload. PR #24 merged.

### M8.3: Remove message builders + msgRef cleanup ✅ — Budget: 8, Used: ~6
Removed all ~22 message builder types. Removed all msgRef types. Direct struct literal construction everywhere. PR #25 merged.

## Upcoming Milestones

### M9: Writeback Cache State + Comp Wrapper Investigation
Populate State for writeback cache (the last component with empty State). Investigate Comp wrapper elimination per human issue #61.

## Resolved Human Issues
- #109: Message builders removed (M8.3)
- #101: GenericMsg removed (M8.2)
- #93: Msg/MsgRef merged (M8.x — Msg is interface, MsgRef eliminated)
- #18: Messages are plain structs with concrete types
- #17: All first-party components ported structurally

## Open Human Issues
- #61: Do we still need Comp struct? — Under investigation

## Known Bugs
- None currently known on main.

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Concrete Types** — `sim.Msg` interface, concrete serializable types per package
4. **Port All Components** — 16 tick-driven components structurally ported (State populated for 15/16; remaining: writeback)
5. **CI Passes** — All checks green on main

## Lessons Learned
- M8.1-M8.3 completed efficiently: multi-worker approach for mechanical changes continues to work well.
- Human feedback drives direction changes (Msg redesign → concrete types → builder removal). Always stay responsive.
- M1-M3 completed in 15 implementation cycles (budgeted 20).
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's/Diana's detailed analysis before milestones prevents scope misestimation.
- M7.1–M7.4 each completed well under budget (12 cycles used vs 24 budgeted).
- When a core design issue surfaces (Payload problem), address it before building more on the broken foundation.
- Total project: ~47 implementation cycles across 97 orchestrator cycles.
