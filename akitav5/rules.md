# Akita V5 Component Rules

This document specifies the authoring rules for Akita V5-style components and what the `akitav5` CLI (`akita`) lints for. The rules are derived from the V5 ideal memory controller (`mem/idealmemcontrollerv5`) and the migration discussion in PR #307.

The goals are:
- Components have a consistent public surface and layout.
- Specs are immutable configuration; state is pure data and serializable.
- Execution is driven by ticking + middlewares rather than ad-hoc events.
- Ports, control operations, and backpressure are explicit.

## Files and Required Types

- `comp.go`
  - Must define a `type Comp struct { ... }` (required by linter).
  - `Comp` must embed `*sim.TickingComponent` and `sim.MiddlewareHolder`.
  - `func (c *Comp) Tick() bool` must delegate to `MiddlewareHolder.Tick()`.
  - Optional but recommended: `SnapshotState() any` and `RestoreState(any) error` to enable simulation snapshot/restore.

- `builder.go`
  - Must define a `type Builder struct { ... }` (required).
  - Must include at least `Engine` and `Freq` fields (required). Additional fields are allowed.
  - Must provide setter methods named `With...` for configurable fields; each must return a `Builder` value (not pointer) so setters can be chained (required by linter).
  - Must define `func (b Builder) Build(name string) *Comp` (required signature and return type).
  - The `Build` function is responsible for constructing `*Comp`, creating the ticking component, and registering middlewares.

- `state.go`
  - Define internal runtime `state` in pure-data form: only primitives, slices, and structs that are JSON-friendly. Avoid embedding simulation objects or pointers to messages.
  - If necessary, provide helper types (e.g., `txn`) that hold only serializable data.

- `spec.go`
  - Define a public immutable `Spec` containing configuration only (e.g., `Width`, `LatencyCycles`, `Freq`, IDs of external resources).
  - Provide defaults and validation: `func defaults() Spec` and `func (s Spec) validate() error`.
  - Any strategy-like configuration (e.g., address conversion) must be expressed as primitive-only spec structs (e.g., `{Kind string, Params map[string]uint64}`) instead of embedding concrete types.

- `middleware files` (e.g., `ctrl_middleware.go`, `mem_middleware.go`)
  - Implement behavior in small, composable middlewares with `Tick() bool` methods.
  - Add middlewares to the component in builder `Build` using `c.AddMiddleware(...)` in the intended execution order.

- `interface.go`
  - Define small local interfaces that abstract external dependencies (e.g., storage, address converter, state accessor). This keeps the component decoupled and easy to test.

- `manifest.json`
  - Required keys: `name` (non-empty string), `ports`, and `parameters` (shape is not enforced by linter, only existence).

## Component Structure

- Embedding and tick delegation
  - `Comp` embeds `*sim.TickingComponent` and `sim.MiddlewareHolder`.
  - `Tick()` returns whether any middleware made progress.

- Ports
  - Components should agree on well-known port aliases. For memory-like components, use `"Top"` for the data path; optionally add `"Control"` for control commands.
  - Ports can be created by the builder or by the caller; tests commonly do:
    - `top := sim.NewPort(comp, inBuf, outBuf, name); comp.AddPort("Top", top)`
    - `ctrl := sim.NewPort(comp, inBuf, outBuf, name); comp.AddPort("Control", ctrl)`
  - Middlewares retrieve ports by alias via `GetPortByName("Top")` and should tolerate missing optional ports (e.g., control) gracefully.

- Middlewares
  - Split responsibilities into focused middlewares:
    - Control middleware: handles enable/pause/drain commands via the `Control` port and updates `state.Mode`. Draining responds when in-flight work finishes.
    - Data-path middleware: implements request intake and completion with countdowns/backpressure and replies on the `Top` port.
  - Return `true` from a middleware tick if it consumed/produced messages or progressed internal countdowns.

## Spec and Builder Rules

- Spec
  - Only immutable configuration values that can be serialized should live in `Spec`.
  - Prefer primitive fields; use nested spec structs with `Kind` + `Params` when modeling strategies.
  - Validate with `validate()`; construct defaults with `defaults()` and a `MakeBuilder()` that uses defaults.

- Builder
  - Fields: must include `Engine sim.Engine` and `Freq sim.Freq`; additional fields are allowed (e.g., `Spec`, `Simulation`, IDs for external state).
  - Setters: each configurable field must have a `WithXxx(...) Builder` method returning a value of type `Builder` and named with a `With` prefix.
  - Build signature: `Build(name string) *Comp` is mandatory and must return `*Comp`.
  - `Build` responsibilities:
    - Validate `Spec` (panic or return error upstream) and ensure required dependencies are present.
    - Initialize `TickingComponent` with engine and frequency.
    - Create and register middlewares in the desired order.
    - Initialize initial `state`.
    - Optionally create ports or leave creation to the caller.

## State Rules

- Keep `state` strictly serializable; do not store pointers to simulation objects or messages.
- If you need to remember a transient message (e.g., reply target for a drain command), store only the minimal pure-data hints in `state` and keep the actual message pointer in a non-serializable field on `Comp`.
- Provide `SnapshotState()` that returns a deep copy of `state` and `RestoreState(any)` that accepts `state` or `*state` and restores it.

## Control Protocol

- Control messages (if supported) use a dedicated `Control` port and a common `mem.ControlMsg`-like contract with flags:
  - Enable: transition to enabled and immediately acknowledge.
  - Pause: transition to paused and immediately acknowledge.
  - Drain: transition to draining; acknowledge when all in-flight work completes, then enter paused.

## Linting Rules Enforced by `akita check`

The CLI linter verifies the following:
- `comp.go` contains a `Comp` struct definition.
- `builder.go` contains a `Builder` struct definition with at least two fields including `Freq` and `Engine`.
- Every configurable `Builder` field is settable via a method whose name starts with `With` and that returns a `Builder` value.
- A `Build(name string) *Comp` method exists on `Builder`, takes exactly one `string` parameter, and returns `*Comp`.
- `manifest.json` exists and contains `name`, `ports`, and `parameters` keys.

## Example Skeleton

```
// comp.go
type Comp struct {
    *sim.TickingComponent
    sim.MiddlewareHolder

    Spec  Spec
    state state
}

func (c *Comp) Tick() bool { return c.MiddlewareHolder.Tick() }

func (c *Comp) SnapshotState() any { /* deep copy state */ }
func (c *Comp) RestoreState(s any) error { /* set state */ return nil }

// builder.go
type Builder struct {
    Engine sim.Engine
    Freq   sim.Freq
    Spec   Spec
}

func MakeBuilder() Builder { return Builder{Spec: defaults()} }
func (b Builder) WithEngine(e sim.Engine) Builder { b.Engine = e; return b }
func (b Builder) WithFreq(f sim.Freq) Builder { b.Freq = f; return b }
func (b Builder) WithSpec(s Spec) Builder { b.Spec = s; return b }
func (b Builder) Build(name string) *Comp { /* create comp, add middlewares */ }

// state.go, spec.go: pure-data state and immutable spec with defaults/validate
// ctrl_middleware.go, mem_middleware.go: focused middlewares
```

## Notes

- Ports are not auto-created by the framework; either the builder or the caller should create and register them on `Comp` with stable aliases.
- When integrating with `simv5.Simulation`, prefer `WithSimulation(*simv5.Simulation)` and derive the engine from it to keep component and simulation in sync.
