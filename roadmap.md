# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model with fully serializable State.

## Current Phase: Investigating M9

### M9: Eliminate Comp Wrapper — Use modeling.Component Directly (INVESTIGATING)

Human issue #145: "A component should only have spec, ports, states, middleware and hooks." Can we remove all per-component Comp structs and use `modeling.Component` directly?

**Status**: Research phase. Two analysts (Iris, Diana) are investigating feasibility and risks. Discussion required before any implementation.

**Key questions**:
- Where do live runtime objects (pipelines, buffers, directory, MSHR) live if not on Comp?
- Can ports be accessed by name instead of stored as fields?
- What design pattern replaces Comp for holding middleware-shared runtime state?
- What are the performance and API implications?

## ✅ Previous Milestones Complete

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2
### M5: Redesign Messages as Plain Structs ✅ — Budget: 8, Used: 6

Replaced `sim.Msg` interface with concrete `Msg` struct. All 31 message types converted to payload structs. PR #14 merged.

### M6: Port All First-Party Components ✅

#### M6.1–M6.4: Ported all 16 tick-driven components structurally ✅ — Budget: 16, Used: 8

### M7: Move Mutable Runtime Data into State Structs ✅

#### M7.1: Simple components (no queueing/cache deps) ✅ — Budget: 6, Used: 4
Populated State for: mmuCache, mmu, gmmu, addresstranslator, datamover, endpoint.

#### M7.2: Components with queueing deps ✅ — Budget: 6, Used: 4
Added queueing snapshot functions (SnapshotPipeline/RestorePipeline/SnapshotBuffer/RestoreBuffer). Populated State for: simplebankedmemory, switches, TLB. PR #20 merged.

#### M7.3: DRAM State Population ✅ — Budget: 6, Used: 2
Populated State for DRAM with full pointer-graph flattening (527-line state.go). PR #21 merged.

#### M7.4: Cache State Population (writearound, writeevict, writethrough) ✅ — Budget: 6, Used: 2
Shared Directory/MSHR serialization helpers in v5/mem/cache/state_helpers.go. State population for 3 near-identical cache variants. PR #22 merged.

#### M7.5: Writeback Cache State Population ✅ — Budget: 6, Used: 4
Refactored custom bufferImpl → queueing.Buffer. Created 937-line state.go with full snapshot/restore. PR #26 merged.

### M8: Msg-as-Interface Redesign ✅

Per human direction (#93), `sim.Msg` became an interface with concrete message types per package.

#### M8.1: Foundation ✅ — Budget: 8, Used: 3
Renamed `Msg` → `GenericMsg`, added `Msg` interface. PR #23 merged.

#### M8.2: Convert all payloads to concrete types + remove GenericMsg ✅ — Budget: 8, Used: 9
30 concrete message types, removed GenericMsg/MsgPayload/TryMsgPayload. PR #24 merged.

#### M8.3: Remove message builders + msgRef cleanup ✅ — Budget: 8, Used: ~6
Removed all ~22 message builder types. Removed all msgRef types. Direct struct literal construction everywhere. PR #25 merged.

## Resolved Human Issues
- #109: Message builders removed (M8.3)
- #101: GenericMsg removed (M8.2)
- #93: Msg/MsgRef merged (M8.x — Msg is interface, MsgRef eliminated)
- #61: Comp wrapper investigation complete — Comp can be simplified but not eliminated (ValidateState constraints). The current Comp+State pattern is architecturally sound.
- #18: Messages are plain structs with concrete types
- #17: All first-party components ported with fully populated State structs (16/16)

## Summary Statistics
- **Total implementation cycles**: ~51 across 102 orchestrator cycles
- **Total milestones**: 8 root milestones (M1–M8), 12 sub-milestones
- **PRs merged**: 26
- **Components fully ported**: 16/16 with serializable State
- **Concrete message types**: 30+
- **Builder types removed**: ~22
- **Lines of state serialization code**: ~4,000+

## Lessons Learned
- Multi-worker approach for mechanical changes works very well (M6, M7, M8)
- Human feedback drives direction changes — stay responsive
- Apollo's verification catches real issues — always verify
- Detailed analysis before milestones prevents scope misestimation
- M7.1–M7.5 each completed well under budget (16 cycles used vs 30 budgeted)
- Breaking large milestones into sub-milestones with 2-6 cycle budgets is optimal
- When a core design issue surfaces, address it before building more on the broken foundation
