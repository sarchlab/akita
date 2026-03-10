# Project Spec

## What to Build

We are evolving the Akita V5 simulation framework. The work has several major threads:

### 1. Component Model (DONE)

Redefine a component as a combination of **Spec, State, Ports, and Middlewares** (see `/v5/migration.md`). A `modeling` package provides `Component[S,T]` — a generic component parameterized by Spec and State types. Builders use `WithSpec()` instead of individual `With*` parameter methods.

### 2. Simulation Save/Load (DONE)

The `simulation` package has `Save(filename)` and `Load(filename)` methods for quiescent-only checkpointing. Components implement `StateSaver`/`StateLoader` interfaces. An acceptance test (`TestSaveLoadDeterminism`) verifies deterministic save/load/resume.

### 3. Messages as Concrete Types (IN PROGRESS — redesign)

**Current state:** `sim.Msg` is a concrete struct with `MsgMeta` + `Payload any`. The `Payload any` field makes messages non-serializable and requires runtime type assertions (`MsgPayload[T]()`).

**Target state (per human direction in #93):** `sim.Msg` becomes an **interface** with a single `Meta() *MsgMeta` method. Each package defines concrete, serializable message types (e.g., `mem.ReadReq`, `cache.FlushReq`) that embed `sim.MsgMeta`. Messages are naturally serializable — no `Payload any`, no `MsgRef`, no runtime casting. Components type-switch on the concrete type: `case *mem.ReadReq:` instead of `MsgPayload[T]()`.

**Design:** See `workspace/iris/msg_as_interface_design.md` for full spec. Key: minimal interface (`Meta() *MsgMeta`), shared `MsgMeta` base struct with pointer receiver, builders return concrete types.

### 4. Port All First-Party Components (DONE — structurally ported, but State needs work)

All first-party components (caches, TLB, MMU, DRAM, datamover, banked memory, NOC components, examples, etc.) have been structurally ported to use the `modeling` package's `Component[S,T]` pattern. However, most have empty `State` structs — their mutable runtime data still lives on the wrapper `Comp` struct, making them non-serializable. See thread #6 below.

### 5. CI Must Pass (DONE)

All CI checks must pass on main. This includes linting (golangci-lint), tests (ginkgo), and acceptance tests.

### 6. Eliminate Comp Wrapper / Move Mutable Data into State (NEW)

Human raised issue #61: currently, ported components like TLB have a `Comp` struct wrapping `*modeling.Component[Spec, State]`, but `State` is empty. All actual mutable runtime data (cache sets, MSHR entries, pipeline state, transaction queues, etc.) lives on the `Comp` struct. This means:

- Components cannot actually be serialized via the State mechanism
- The `Comp` wrapper duplicates the role that `State` should play
- The modeling pattern is structurally present but semantically broken

The goal is to move all mutable runtime data into `State` so components are truly serializable, OR redesign the component architecture to eliminate the need for the `Comp` wrapper struct. The key question: **can a component's mutable data (including things with pointers like MSHR, pipelines, buffers) be made serializable?**

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
