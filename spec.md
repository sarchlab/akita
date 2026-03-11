# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework. The work has several major threads:

### 1. Component Model (DONE)

Redefine a component as a combination of **Spec, State, Ports, and Middlewares** (see `/v5/migration.md`). A `modeling` package provides `Component[S,T]` — a generic component parameterized by Spec and State types. Builders use `WithSpec()` instead of individual `With*` parameter methods.

### 2. Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods for quiescent-only checkpointing. Components implement `StateSaver`/`StateLoader` interfaces. An acceptance test (`TestSaveLoadDeterminism`) verifies deterministic save/load/resume.

### 3. Messages as Concrete Types (DONE)

`sim.Msg` is an interface with `Meta() *MsgMeta`. Each package defines concrete, serializable message types (e.g., `mem.ReadReq`, `cache.FlushReq`) embedding `sim.MsgMeta`. No `Payload any`, no `GenericMsg`, no runtime casting, no message builders, no msgRef types. Components type-switch on concrete types: `case *mem.ReadReq:`. Messages are constructed as plain struct literals.

### 4. Port All First-Party Components (DONE)

All first-party components have been structurally ported to use the `modeling` package's `Component[S,T]` pattern. State is fully populated for all 16 components with meaningful, serializable State structs.

### 5. CI Must Pass (DONE)

All CI checks must pass on main. This includes linting (golangci-lint), tests (ginkgo), and acceptance tests.

### 6. Component Creation Guide (IN PROGRESS)

Human raised issue #148: Write a guide on how to create a component in V5. This guide should document the current component model including Spec, State, Ports, Middleware, Hooks, and the Builder pattern.

### 7. Eliminate Comp Wrapper — Use modeling.Component Directly (DISCUSSION)

Human raised issue #145: "A component should only have spec, ports, states, middleware and hooks. Can we just remove all the components struct definition from all the individual components and use modeling.component instead?"

Human wants to discuss before coding. The goal is to eliminate all per-component `Comp` structs so that `modeling.Component[Spec, State]` IS the component — no wrapper needed. This means:
- Ports should be accessible from modeling.Component (it already inherits PortOwnerBase)
- All mutable runtime data should live in State (or be reconstructable from State+Spec)
- Live runtime objects (pipelines, buffers, etc.) should be managed by middleware or reconstructed as needed
- No duplicated fields between Comp and State

Analysis complete. Awaiting human feedback on design proposal before implementing.

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
