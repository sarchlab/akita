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

#### M7.1: Simple components (no queueing) ✅ — Budget: 6, Used: 2
Populated State for 6 components: mmuCache, mmu, gmmu, addresstranslator, datamover, endpoint. All validated by Apollo. PR #19 merged.

## Current Milestone

### M7.2: Queueing snapshot + simplebankedmemory, switches, TLB — Budget: 6

**Approach decided (cycle 63):** Add standalone `SnapshotPipeline()`/`RestorePipeline()`/`SnapshotBuffer()`/`RestoreBuffer()` functions to the queueing package. These use type assertions to access unexported `pipelineImpl`/`bufferImpl` internals. No interface changes = no mock file updates needed.

Then populate State for 3 components using the established endpoint pattern:
1. **simplebankedmemory** — bank pipeline/buffer contents → serializable state
2. **switches** — per-portComplex pipeline/buffer/routing state → serializable state
3. **TLB** — sets, MSHR, pipeline, buffer, inflightFlushReq → serializable state

## Upcoming Milestones

### M7.3: DRAM State serialization — Budget: TBD
Deep internal packages (signal, cmdq, trans, org). Pointer graph flattening with index-based references. ~300-400 lines. Independent of queueing.

### M7.4: Cache variants State (writearound, writeevict, writethrough) — Budget: TBD
Shared code for 3 near-identical cache variants. Directory, MSHR, transaction, stage serialization. ~400-500 lines shared.

### M7.5: Writeback cache State — Budget: TBD
Most complex cache variant. Additional write buffer, flusher, more actions. ~600-800 lines. Reuses Directory/MSHR serialization from M7.4.

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
- Total project so far: ~29 implementation cycles + ~12 planning/verification cycles across 63 orchestrator cycles.
- **Key decision (cycle 63)**: Use standalone functions (not interface methods) to snapshot/restore queueing internals. Avoids 9 mock file updates. Type assertions to concrete impl are fine since we control all implementations.
- The `*sim.Msg` pointer problem is universal — the endpoint decomposition pattern is proven.
- Diana's Approach A (pragmatic) + standalone functions variant = lowest risk path to unblock all 7 queueing-dependent components.
