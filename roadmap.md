# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model.

## Completed Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2

## Current Milestone

### M5: Redesign Messages as Plain Structs — Budget: 8
**Goal:** Replace the `sim.Msg` interface with a concrete `Msg` struct. Eliminate `sim.Rsp`, `sim.Request` interfaces and all per-type `Meta()`/`Clone()` boilerplate. Introduce `Payload any` field for domain-specific data and generic helpers `MsgPayload[T]`/`TryMsgPayload[T]`.

**Design:** Based on Iris's analysis (issue #24). Key changes:
- `sim.Msg` becomes a concrete struct with embedded `MsgMeta`, `RspTo string`, `Payload any`
- Port interface changes from `Send(Msg)` to `Send(*Msg)`, etc.
- All 31 message types become payload structs (e.g., `ReadReqPayload`, `WriteReqPayload`)
- Type dispatch changes from `switch msg.(type)` to `switch msg.Payload.(type)`
- `msg.Meta().X` becomes `msg.X` (direct embed access)
- `msg.(sim.Rsp).GetRspTo()` becomes `msg.RspTo`
- Scope: ~49 production files + ~30 test/mock files

**Status:** Starting implementation

## Upcoming Milestones

### M6: Port all first-party components to modeling package
**Goal:** Port all remaining 15 components to `modeling.Component[S,T]`:
- Batch 1 (easy): examples/tickingping, examples/ping
- Batch 2 (simple): datamover, simplebankedmemory, gmmu
- Batch 3 (medium): mmu, addresstranslator, noc/endpoint, noc/switches
- Batch 4 (complex): tlb, mmuCache, writearound, writeevict, writethrough, writeback
- Plus DRAM and test agents
**Budget:** TBD (40-56 cycles estimated, will break into sub-milestones)
**Status:** Pending M5 completion

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20). Planning and verification added ~3-4 cycles overhead but improved quality.
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's detailed analysis before M3 was valuable — prevents scope misestimation.
- CI lint rules (funlen, gocognit) must be respected — fix immediately, don't accumulate tech debt.
- Breaking work across parallel workers (Kai, Nova, Leo) speeds implementation significantly.
- M4 completed efficiently in 2 cycles (budgeted 3) — lint fixes are mechanical once identified.
- Research cycles (Diana, Iris) before M5/M6 prevented committing to poorly scoped milestones.
- Message redesign is foundational — must complete before component porting (M6) to avoid double work.
