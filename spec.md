# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework. The work has several major threads:

### 1. Component Model (DONE)

Redefine a component as a combination of **Spec, State, Ports, and Middlewares** (see `/v5/migration.md`). A `modeling` package provides `Component[S,T]` — a generic component parameterized by Spec and State types. Builders use `WithSpec()` instead of individual `With*` parameter methods.

### 2. Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods for quiescent-only checkpointing. Components implement `StateSaver`/`StateLoader` interfaces. An acceptance test (`TestSaveLoadDeterminism`) verifies deterministic save/load/resume.

### 3. Messages as Plain Structs (NEW)

The `Msg` interface in `sim/msg.go` should be replaced with a plain struct. Design a new message interface/struct that eliminates the need for every message type to implement `Meta()`, `Clone()`, etc. methods manually. Messages should be simple, serializable data. Also design the new interface for messages.

### 4. Port All First-Party Components (NEW)

All first-party components (caches, TLB, MMU, DRAM, datamover, banked memory, NOC components, examples, etc.) must be ported to use the `modeling` package's `Component[S,T]` pattern with Spec/State/Middlewares. Currently only `idealmemcontroller` has been ported.

### 5. CI Must Pass (NEW)

All CI checks must pass on main. This includes linting (golangci-lint), tests (ginkgo), and acceptance tests.

## Success Criteria

- Simple, straightforward, intuitive APIs
- All CI checks pass on main branch
- Acceptance test for save/load process passes
- All first-party components use the modeling package pattern
- Messages are plain structs, not interfaces

## Constraints

- Follow the patterns described in `/v5/migration.md`
- Keep State pure and serializable (no pointers, live handles, functions, channels)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
