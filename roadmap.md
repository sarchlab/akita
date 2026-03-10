# Roadmap

## Project Goal

Redefine the component model in Akita V5 following the Spec/State/Ports/Middlewares pattern described in `migration.md`. Create a `modeling` package, refactor builders to use `WithSpec`, implement simulation save/load (serialization), and create an acceptance test for save/load.

## Milestones

### M1: Create `modeling` package with Component struct (Spec/State pattern) — Budget: 6 cycles
**Status:** Not started

Define the core `modeling` package at `v5/modeling/`. This package provides:
- A generic `Component[S Spec, T State]` struct that holds Spec, State, Ports, Middlewares
- `Spec` and `State` interfaces/constraints ensuring serializability (primitives only)
- A `Builder` that accepts a `Spec` via `WithSpec()` 
- Integration with the existing `sim.TickingComponent` and `sim.MiddlewareHolder` patterns
- All existing tests must still pass (no regressions)

### M2: Refactor `idealmemcontroller` to use the modeling package — Budget: 6 cycles
**Status:** Not started

Convert `idealmemcontroller` from the current ad-hoc pattern to use the new `modeling` package:
- Define `idealmemcontroller.Spec` (Width, Latency, Freq, Capacity, CacheLineSize, StorageRef, AddrConv)
- Define `idealmemcontroller.State` (pure data: inflightBuffer as serializable data, state enum)
- Builder uses `WithSpec(spec)` instead of many `With*` functions for basic parameters
- Acceptance tests for idealmemcontroller still pass

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
- (none yet)
