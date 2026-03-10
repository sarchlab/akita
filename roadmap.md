# Roadmap

## Project Goal

Redefine the component model in Akita V5 following the Spec/State/Ports/Middlewares pattern described in `migration.md`. Create a `modeling` package, refactor builders to use `WithSpec`, implement simulation save/load (serialization), and create an acceptance test for save/load.

## Milestones

### M1: Create `modeling` package with Component struct (Spec/State pattern) — Budget: 6 cycles
**Status:** ✅ Complete (actual: 3 cycles)

Delivered:
- `Component[S, T]` struct with GetSpec/GetState/SetState, embeds TickingComponent + MiddlewareHolder
- Builder with WithSpec/WithEngine/WithFreq/Build
- ValidateSpec/ValidateState runtime validation
- 12 unit tests + ping-pong example test, all passing
- PR #10 merged to main

### M2: Refactor `idealmemcontroller` to use the modeling package — Budget: 6 cycles
**Status:** In progress (next milestone)

Convert `idealmemcontroller` from the current ad-hoc pattern to use the new `modeling` package:
- Define `Spec` struct: Width, Latency, Freq, CacheLineSize, StorageRef (string ID), AddrConvKind
- Define `State` struct: pure data only — inflight transaction data with countdowns, state enum string
- Replace event-based latency (`readRespondEvent`/`writeRespondEvent`) with tick-driven countdowns per migration.md
- Builder uses `WithSpec(spec)` for basic configuration, keeps `WithStorage()`, `WithTopPort()`, `WithCtrlPort()` for non-serializable dependencies
- Existing unit tests and acceptance tests must still pass
- Component must use `modeling.Component[Spec, State]` instead of manual `*sim.TickingComponent` + `sim.MiddlewareHolder`

### M3: Implement simulation Save/Load — Budget: 6 cycles
**Status:** Not started

Add `Save(filename string)` and `Load(filename string)` methods to the `Simulation` struct:
- Serialize all component Specs and States to a file (JSON or gob)
- On Load, reconstruct components from saved Specs/States
- Engine time is saved/restored
- Storage (mem.Storage) is saved/restored via a state registry

### M4: Save/Load acceptance test — Budget: 4 cycles
**Status:** Not started

Create an acceptance test (similar to existing mem acceptance tests) that:
- Sets up a simulation with an ideal memory controller
- Runs for some steps, saves state
- Loads state into a fresh simulation
- Continues execution and verifies correctness

## Lessons Learned
- M1 completed in 3 cycles (budgeted 6) — the modeling package was straightforward since it's purely additive. Future refactoring milestones (modifying existing code) will likely be harder.
- Apollo merged the PR directly — good for velocity but we should ensure CI passes.
