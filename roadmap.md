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

### M7.3: DRAM State Population ✅ — Budget: 6, Used: 2
Populated State for DRAM with full pointer-graph flattening (527-line state.go). Created internal accessor files. PR #21 merged.

## Upcoming Milestones

### M7.4: Cache State Population (writearound, writeevict, writethrough) — NEXT
Three near-identical cache variants (~90% shared code). Must serialize:
- cache.Directory (Sets with Blocks + LRU ordering)
- cache.MSHR (entries with *sim.Msg decomposition, *Block references, []interface{} Requests)
- Transactions (11 fields each, 5 need pointer decomposition)
- Pipeline/buffer contents (wrap transactions)
- Component-level state (transactions, postCoalesceTransactions, isPaused)

Approach: Create shared Directory/MSHR serialization helpers in v5/mem/cache/ package, then each cache gets its own state.go.

Estimated: ~400-500 lines shared + ~200 per variant.

### M7.5: Writeback Cache State Population
Most complex cache. Additional state beyond M7.4:
- 17-field transaction (vs 11 for others)
- Write buffer stage (pendingEvictions, inflightFetch, inflightEviction)
- MSHR stage state
- Evicting list map
- Cache state enum (running/preFlushing/flushing/paused)

Can reuse Directory/MSHR serialization from M7.4.
Estimated: ~600-800 lines.

## Previously Completed Goals
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components structurally ported (State now populated for 10/15; remaining: 4 cache variants + idealmemcontroller already done)
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
- M7.1 and M7.2 each completed in 4 cycles (budgeted 6) — consistent track record of finishing under budget.
- M7.3 completed in 2 cycles (budgeted 6) — the DRAM pattern is well-understood now, single-worker approach worked well.
- Total project: ~37 implementation cycles across 73 orchestrator cycles. Overhead from planning/verification is worthwhile (catches real bugs).
