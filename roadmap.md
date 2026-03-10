# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model.

## Completed Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2
### M5: Redesign Messages as Plain Structs ✅ — Budget: 8, Used: 6

Replaced `sim.Msg` interface with concrete `Msg` struct. Eliminated `sim.Rsp`, `sim.Request` interfaces and per-type `Meta()`/`Clone()` boilerplate. Added `Payload any` field, `MsgPayload[T]`/`TryMsgPayload[T]` helpers. All 31 message types converted to payload structs. PR #14 merged, verified by Apollo.

## Current Phase: M6 — Port All First-Party Components

### Overall Strategy
Port all 15 remaining components to `modeling.Component[S,T]` pattern. Reference implementation: `mem/idealmemcontroller`. Broken into sub-milestones by complexity.

### M6.1: Port simple components (tickingping, datamover, gmmu) — NEXT
**Goal:** Port 3 simple components to `modeling.Component[S,T]`:
1. `examples/tickingping` — Already has middleware pattern, ~237 LoC, simplest possible port
2. `mem/datamover` — ~639 LoC, needs middleware refactor, 3 ports
3. `mem/vm/gmmu` — ~388 LoC, needs middleware refactor + fix value embed

These establish the porting template for more complex components.

### M6.2: Port medium VM components (mmu, addresstranslator, mmuCache)
**Goal:** Port 3 VM components using patterns from M6.1
- `mem/vm/mmu` — Migration state machine, 587 LoC
- `mem/vm/addresstranslator` — 4 ports, interface-typed transactions, 729 LoC
- `mem/vm/mmuCache` — Internal set serialization, 874 LoC

### M6.3: Port TLB + simplebankedmemory
**Goal:** Port TLB (1234 LoC) and simplebankedmemory (510 LoC)
- Both depend on `queueing.Pipeline`/`Buffer` — may need serialization strategy
- TLB has custom MSHR + internal sets

### M6.4: Port NOC components (endpoint, switches)
- `noc/networking/switching/endpoint` — 497 LoC, dynamic ports
- `noc/networking/switching/switches` — 429 LoC, routing table

### M6.5: Port cache components (writearound, writeevict, writethrough, writeback)
- Prerequisite: serializable `cache.Directory` and `cache.MSHR`
- Most complex batch, ~7200 LoC total
- Port writearound first as template

### M6.6: Port DRAM
- `mem/dram` — 2154 LoC, complex internal subsystems

### M6.7: Special cases
- `examples/ping` — Event-driven (not ticking), needs architectural decision
- Test agents — Low priority

## Open Human Issues
- #16: Fix CI ✅ (done in M4, monitoring)
- #17: Port all first-party components → M6 (in progress)
- #18: Message should be plain struct → M5 ✅ (done)

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20). Planning and verification added ~3-4 cycles overhead but improved quality.
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's detailed analysis before M3 was valuable — prevents scope misestimation.
- CI lint rules (funlen, gocognit) must be respected — fix immediately, don't accumulate tech debt.
- Breaking work across parallel workers (Kai, Nova, Leo) speeds implementation significantly.
- M4 completed efficiently in 2 cycles (budgeted 3) — lint fixes are mechanical once identified.
- Research cycles (Diana, Iris) before M5/M6 prevented committing to poorly scoped milestones.
- Message redesign is foundational — completed before component porting (M6) to avoid double work.
- M5 completed in 6 implementation cycles (budgeted 8). Multi-worker parallel approach was highly effective for mechanical refactoring.
