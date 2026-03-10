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

### M6.3: Port TLB + simplebankedmemory + NOC components ✅ — Budget: 4, Used: 2
Ported 4 components: TLB, simplebankedmemory, endpoint, switches. PR #17 merged, verified by Apollo.

### M6.4: Port cache components + DRAM — NEXT
**Goal:** Port 4 cache components and DRAM memory controller to modeling.Component[S,T]
- `mem/cache/writearound` — 4324 LoC total, same pattern as writeevict/writethrough
- `mem/cache/writeback` — 6359 LoC total, largest cache, has write buffer stage
- `mem/cache/writeevict` — 4287 LoC total
- `mem/cache/writethrough` — 4538 LoC total
- `mem/dram` — 2057 LoC, internal subsystems stay on Comp

All 5 have `*sim.TickingComponent` + `sim.MiddlewareHolder` already. Porting pattern is identical to previous milestones: replace with `*modeling.Component[Spec, State]`, create empty/minimal Spec/State, update builders. `cache.Directory` and `cache.MSHR` are interfaces — stay on Comp, not in Spec/State.

### M6.5: Special cases (lower priority)
- `examples/ping` — Event-driven (uses `ComponentBase` not `TickingComponent`), needs architectural decision
- Test agents (`noc/standalone/agent`, `noc/acceptance/agent`, `mem/acceptancetests/memaccessagent`) — Low priority, test infrastructure
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
- M6.2 completed in 2 impl cycles (budgeted 4). Same parallel pattern works.
- M6.3 completed in 2 impl cycles (budgeted 4). 4 components ported in parallel, verified by Apollo. Consistent efficiency.
