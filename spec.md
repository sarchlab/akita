# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework. The work has several major threads:

### 1. Component Model (DONE)

Redefine a component as a combination of **Spec, State, Ports, and Middlewares** (see `/v5/migration.md`). A `modeling` package provides `Component[S,T]` — a generic component parameterized by Spec and State types. Builders use `WithSpec()` instead of individual `With*` parameter methods.

### 2. Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods for quiescent-only checkpointing. Components implement `StateSaver`/`StateLoader` interfaces. An acceptance test (`TestSaveLoadDeterminism`) verifies deterministic save/load/resume.

### 3. Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types (e.g., `mem.ReadReq`, `cache.FlushReq`) embedding `sim.MsgMeta`. No `Payload any`, no `GenericMsg`, no runtime casting, no message builders, no msgRef types. Components type-switch on concrete types: `case *mem.ReadReq:`. Messages are constructed as plain struct literals.

### 4. Port All First-Party Components (DONE — structurally ported, State needs work for writeback)

All first-party components have been structurally ported to use the `modeling` package's `Component[S,T]` pattern. State is populated for 15 of 16 components. Only writeback cache has an empty State struct.

### 5. CI Must Pass (DONE)

All CI checks must pass on main. This includes linting (golangci-lint), tests (ginkgo), and acceptance tests.

### 6. Eliminate Comp Wrapper / Move Mutable Data into State (IN PROGRESS)

Human raised issue #61: currently, ported components like TLB have a `Comp` struct wrapping `*modeling.Component[Spec, State]`, but mutable runtime data is duplicated — it exists on both the `Comp` struct (as live objects) and in `State` (as serializable snapshots). SaveState copies Comp→State, LoadState restores State→Comp.

The goal is to investigate whether the `Comp` wrapper can be simplified or eliminated, reducing duplication between live objects and their serializable representations.

## Success Criteria

- Simple, straightforward, intuitive APIs
- All CI checks pass on main branch
- Acceptance test for save/load process passes
- All first-party components use the modeling package pattern
- Messages are concrete, serializable types behind a `sim.Msg` interface
- All first-party components have meaningful, serializable State structs (no empty State with data hidden on Comp)

## Constraints

- Follow the patterns described in `/v5/migration.md`
- Keep State pure and serializable (no pointers, live handles, functions, channels)
- Keep Spec primitive and JSON-friendly
- Use tick-driven patterns; prefer countdowns over scheduled events
