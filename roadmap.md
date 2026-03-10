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

### M6.1: Port simple components (tickingping, datamover, gmmu) ✅ — Budget: 4, Used: 2

Ported 3 simple components to `modeling.Component[S,T]` pattern. All tests pass, ValidateSpec/ValidateState pass, PR #15 merged. Established the porting template.

## Current Phase: M6 — Port All First-Party Components

### Overall Strategy
Port remaining ~12 components to `modeling.Component[S,T]` pattern. Reference implementations: `mem/idealmemcontroller`, plus the 3 just ported in M6.1.

### M6.2: Port medium VM components (mmu, addresstranslator, mmuCache) ✅ — Budget: 4, Used: 2

Ported 3 VM components to `modeling.Component[S,T]` pattern. All tests pass, ValidateSpec/ValidateState pass, PR #16 merged. All 3 had MiddlewareHolder already.

### M6.3: Port TLB + simplebankedmemory + NOC components — NEXT
**Goal:** Port 4 components: TLB, simplebankedmemory, endpoint, switches
- `mem/vm/tlb` — 2380 LoC, custom MSHR + internal sets. Already has MiddlewareHolder.
- `mem/simplebankedmemory` — 926 LoC. Already has MiddlewareHolder.
- `noc/networking/switching/endpoint` — dynamic ports. Already has MiddlewareHolder.
- `noc/networking/switching/switches` — routing table. Already has MiddlewareHolder.

All use MiddlewareHolder pattern; same mechanical porting as M6.1/M6.2.

### M6.4: Port cache components (writearound, writeevict, writethrough, writeback)
- Most complex batch, ~19500 LoC total (incl. tests)
- Prerequisite: serializable `cache.Directory` and `cache.MSHR`
- Port writearound first as template

### M6.5: Port DRAM
- `mem/dram` — 2057 LoC, complex internal subsystems

### M6.6: Special cases
- `examples/ping` — Event-driven (not ticking), needs architectural decision
- Test agents (`noc/standalone/agent`, `noc/acceptance/agent`, `mem/acceptancetests/memaccessagent`) — Low priority
- `sim/directconnection` — Connection, not standard component. May or may not need porting.

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
- M6.1 completed very efficiently in 2 implementation cycles (budgeted 4). Parallel 3-worker approach with clear instructions works well for mechanical porting.
- Components that already have MiddlewareHolder are easier to port — the middleware pattern is already there.
