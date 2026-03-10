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

### M6: Port All First-Party Components ✅

#### M6.1: Port simple components (tickingping, datamover, gmmu) ✅ — Budget: 4, Used: 2
#### M6.2: Port medium VM components (mmu, addresstranslator, mmuCache) ✅ — Budget: 4, Used: 2
#### M6.3: Port TLB + simplebankedmemory + NOC components ✅ — Budget: 4, Used: 2
#### M6.4: Port cache components + DRAM ✅ — Budget: 4, Used: 2

Ported all 16 tick-driven first-party components to `modeling.Component[S,T]`:
- `examples/tickingping`
- `mem/datamover`, `mem/idealmemcontroller`, `mem/simplebankedmemory`, `mem/dram`
- `mem/cache/writearound`, `mem/cache/writeback`, `mem/cache/writeevict`, `mem/cache/writethrough`
- `mem/vm/gmmu`, `mem/vm/mmu`, `mem/vm/addresstranslator`, `mem/vm/mmuCache`, `mem/vm/tlb`
- `noc/networking/switching/endpoint`, `noc/networking/switching/switches`

Remaining non-ported files are infrastructure/test code (not first-party components):
- `examples/ping` — Event-driven example; `tickingping` is its modeling replacement
- `mem/acceptancetests/memaccessagent` — Test utility agent
- `noc/acceptance/agent`, `noc/standalone/agent` — Test utility agents
- `sim/directconnection` — Connection infrastructure, not a component
- `mem/cache/mshr` — Internal cache data structure

## Project Status: COMPLETE ✅

All human-requested goals achieved:
1. **Component Model** — `modeling.Component[S,T]` with Spec/State/Ports/Middlewares
2. **Save/Load** — `simulation.Save()`/`Load()` with deterministic acceptance test
3. **Messages as Plain Structs** — `sim.Msg` is concrete struct with typed payloads
4. **Port All Components** — 16 tick-driven components ported
5. **CI Passes** — All checks green on main

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
- M6 sub-milestones consistently completed in 2 cycles each (budgeted 4). Parallel multi-worker approach with clear instructions is the winning pattern for mechanical porting.
- Components that already have MiddlewareHolder are easier to port.
- Total project: ~27 implementation cycles + ~10 planning/verification cycles = ~37 active cycles across 54 orchestrator cycles.
