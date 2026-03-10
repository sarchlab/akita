# Roadmap

## Project Goal

Evolve Akita V5: redefine component model, implement save/load, make messages plain structs, and port all first-party components to the new model.

## Completed Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6, Used: 5
### M2: Refactor `idealmemcontroller` to use modeling package ✅ — Budget: 6, Used: 4
### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8, Used: 6
### M4: Fix CI lint failures ✅ — Budget: 3, Used: 2

## Current Phase: Research & Design

Gathering analysis before committing to M5 milestone. Two research tasks in progress:
- Issue #24: Design analysis for messages as plain structs (assigned to Iris)
- Issue #25: Component porting scope and dependency order analysis (assigned to Diana)

## Upcoming Milestones

### M5: Redesign Messages as Plain Structs
**Goal:** Replace the `Msg` interface with a plain struct. Design a new message type that doesn't require every message to implement `Meta()`, `Clone()` etc. Update all first-party message types and their callers. This is a foundational change that should happen BEFORE porting components (M6), since the ported components will use the new message type.
**Budget:** TBD (pending design analysis)
**Status:** Design analysis in progress

### M6: Port all first-party components to modeling package
**Goal:** Port all remaining components to `modeling.Component[S,T]`:
- mem/cache/* (writethrough, writeback, writeevict, writearound)
- mem/simplebankedmemory
- mem/dram
- mem/datamover
- mem/vm/* (tlb, mmuCache, gmmu, mmu, addresstranslator)
- noc/networking/switching/* (switches, endpoint)
- noc/standalone/agent
- examples/ping, examples/tickingping
**Budget:** TBD (pending scope analysis)
**Status:** Scope analysis in progress

## Lessons Learned
- M1-M3 completed in 15 implementation cycles (budgeted 20). Planning and verification added ~3-4 cycles overhead but improved quality.
- Apollo's verification caught 4 real issues in M2 — always verify.
- Iris's detailed analysis before M3 was valuable — prevents scope misestimation.
- CI lint rules (funlen, gocognit) must be respected — fix immediately, don't accumulate tech debt.
- Breaking work across parallel workers (Kai, Nova, Leo) speeds implementation significantly.
- M4 completed efficiently in 2 cycles (budgeted 3) — lint fixes are mechanical once identified.
