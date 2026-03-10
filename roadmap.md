# Roadmap

## Project Goal

Redefine the component model in Akita V5 following the Spec/State/Ports/Middlewares pattern described in `migration.md`. Create a `modeling` package, refactor builders to use `WithSpec`, implement simulation save/load (serialization), and create an acceptance test for save/load.

## Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6 cycles, Used: 5
**Status:** Complete (PR #10 merged to main)

### M2: Refactor `idealmemcontroller` to use the modeling package ✅ — Budget: 6 cycles, Used: 4
**Status:** Complete (PR #11 merged to main via PR #12)

### M3: Implement simulation Save/Load with acceptance test ✅ — Budget: 8 cycles, Used: 6
**Status:** Complete (PR #12 merged to main)

Implemented `Simulation.Save(filename)` and `Simulation.Load(filename)` with quiescent-only checkpointing. Includes acceptance test `TestSaveLoadDeterminism` that saves mid-simulation, loads, and verifies deterministic continuation.

Key deliverables:
- `v5/modeling/saveload.go` — SaveState/LoadState on Component[S,T]
- `v5/mem/mem/storage_saveload.go` — Save/Load on Storage
- `v5/sim/ticker.go` — ResetTickScheduler method
- `v5/sim/idgenerator.go` — Get/SetNextID
- `v5/simulation/saveload.go` — Simulation.Save/Load orchestration
- `v5/sim/port.go` — Port buffer access methods
- `v5/mem/acceptancetests/saveload/` — Acceptance test

## Project Complete ✅

All milestones achieved. Total cycles used: 18 (across all phases including planning, implementation, and verification).

## Lessons Learned
- M1 and M2 went faster than budgeted (5 and 4 cycles vs 6 each). Good velocity.
- Apollo's verification caught 4 real issues in M2 — the verify step adds ~2-3 cycles but prevents bad code from landing.
- Iris's detailed analysis before M3 planning was very valuable — prevents scope misestimation.
- Breaking down the problem (quiescent-only first, non-quiescent later) keeps milestones achievable.
- Parallel worker assignment (Kai for sim/, Nova for modeling+mem/, Leo for integration) significantly speeds up implementation.
- Budget: 20 total budgeted cycles for M1-M3, used 15 implementation cycles. Planning and verification add overhead (~3-4 cycles) but improve quality.
