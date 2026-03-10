# Roadmap

## Project Goal

Redefine the component model in Akita V5 following the Spec/State/Ports/Middlewares pattern described in `migration.md`. Create a `modeling` package, refactor builders to use `WithSpec`, implement simulation save/load (serialization), and create an acceptance test for save/load.

## Milestones

### M1: Create `modeling` package with Component struct ✅ — Budget: 6 cycles, Used: 5
**Status:** Complete (PR #10 merged to main)

### M2: Refactor `idealmemcontroller` to use the modeling package ✅ — Budget: 6 cycles, Used: 4
**Status:** Complete (PR #11 merged to ares/m1-modeling-package)

Note: M2 code lives on `ares/m1-modeling-package` branch, not yet merged to `main`. M3 should build on that branch.

### M3: Implement simulation Save/Load with acceptance test — Budget: 8 cycles
**Status:** In planning

Implement `Simulation.Save(filename)` and `Simulation.Load(filename)` with quiescent-only checkpointing (save when no in-flight messages). Includes an acceptance test that saves mid-simulation, loads, and verifies deterministic continuation.

Key design decisions (from Iris's analysis):
- Quiescent-only saves (port buffers empty at save time) — avoids message serialization complexity
- `modeling.Component[S,T].SaveState/LoadState` using JSON (Spec/State already constrained to be serializable)
- `mem.Storage.Save/Load` for memory data
- Engine time + ID generator state saved/restored
- Event queue NOT serialized — reconstructed via `TickLater()` after restore
- Connections NOT serialized — reconstructed from build code
- TickScheduler reset after restore

Files to create/modify (~800-1100 lines):
- `v5/modeling/saveload.go` — SaveState/LoadState on Component[S,T]
- `v5/mem/mem/storage_saveload.go` — Save/Load on Storage
- `v5/sim/ticker.go` — ResetTickScheduler method
- `v5/sim/idgenerator.go` — Get/SetNextID
- `v5/simulation/saveload.go` — Simulation.Save/Load orchestration
- `v5/sim/port.go` — Port buffer access methods (for quiescent verification)
- Acceptance test: save/load test with idealmemcontroller

## Lessons Learned
- M1 and M2 went faster than budgeted (5 and 4 cycles vs 6 each). Good velocity.
- Apollo's verification caught 4 real issues in M2 — the verify step adds ~2-3 cycles but prevents bad code from landing.
- Iris's detailed analysis before M3 planning was very valuable — prevents scope misestimation.
- Breaking down the problem (quiescent-only first, non-quiescent later) keeps milestones achievable.
