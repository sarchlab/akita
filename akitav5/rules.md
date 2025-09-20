# Akita V5 Component Rules

This document defines numbered rules for Akita V5-style components and what the `akita` CLI lints for. 

1. Component
  1.1 A package (directory) defines at most one component.
  1.2 A component package must include a `//akita:component` comment (no space after the slashes) to mark the root.
  1.3 Must define a `type Comp struct { ... }` in `comp.go`.

2. State
  2.1 Must define `state` struct with pure-data. Primitives are always OK. List and maps are OK as long as the values are pure data. Structs can also be considered pure data if the internal fields are all pure data. No pointers to other elements. (No interfaces, channels, functions, etc.)

1. Spec
  3.1 Must define an immutable `Spec` containing only configuration.
  3.2 Must provide `func defaults() Spec` with sane defaults.
  3.3 Must provide `func (s Spec) validate() error` for runtime validation.
  3.4 Should model strategies via primitive-only spec structs, e.g., `{Kind string, Params map[string]uint64}`.

1. Middleware
  4.1 Must decompose behavior into focused middlewares with `Tick() bool`.
  4.2 Must register middlewares in `Builder.Build` using `c.AddMiddleware(...)` in execution order.
  4.3 Control middleware Should handle enable, pause, and drain via a `Control` port, updating `state.Mode` and responding appropriately when in-flight work completes.
  4.4 Data-path middleware Should implement request intake, countdown progress, and responses with port backpressure retries.

1. Builder
  5.1 Must define a `type Builder struct { ... }` in `builder.go`. (Linter: yes)
  5.2 Must include fields `Engine sim.Engine` and `Freq sim.Freq`. (Linter: yes)
  5.3 Must provide `WithXxx(...) Builder` setters for configurable fields; method names start with `With`. (Linter: yes)
  5.4 Each `WithXxx` Must return a `Builder` value (not pointer) to support chaining. (Linter: yes)
  5.5 Must define `func (b Builder) Build(name string) *Comp`. (Linter: yes)
  5.6 `Build` Must take exactly one parameter of type `string`. (Linter: yes)
  5.7 `Build` Must return a `*Comp`. (Linter: yes)
  5.8 `Build` Must initialize `TickingComponent` with engine and frequency, register middlewares, and initialize State.
  5.9 `MakeBuilder()` Should return a Builder with `defaults()` applied.

1. Ports
  6.1 Should use stable port aliases. For memory-like components: `"Top"` for data path and optional `"Control"` for control commands.
  6.2 Ports May be created by the builder or by the caller and added via `AddPort(alias, port)`.
  6.3 Middlewares Must retrieve ports by alias via `GetPortByName(alias)` and Should tolerate missing optional ports gracefully (e.g., control absent).

1. Linter Coverage (current `akita component-lint`)
  8.1 Enforced: 1.2, 1.3, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7.
  8.2 Not yet enforced (documented for authorship and future checks): 1.1, 2.1–2.3, 3.1–3.4, 4.1–4.4, 5.8–5.9, 6.1–6.3, 7.1–7.3.

1. Example Skeleton
  9.1 `comp.go` example
  
  ```go
  type Comp struct {
      *sim.TickingComponent
      sim.MiddlewareHolder
      Spec  Spec
      state state
  }
  func (c *Comp) Tick() bool { return c.MiddlewareHolder.Tick() }
  func (c *Comp) SnapshotState() any { /* deep copy state */ }
  func (c *Comp) RestoreState(s any) error { /* set state */ return nil }
  ```

  9.2 `builder.go` example
  
  ```go
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
  ```

  9.3 Notes
  
  - Place `//akita:component` once in the package (commonly in `doc.go`) so tooling can discover the component.
  - Ports are not auto-created by the framework; either the builder or the caller should create and register them on `Comp` with stable aliases.
  - When integrating with `simv5.Simulation`, prefer `WithSimulation(*simv5.Simulation)` and derive the engine from it to keep component and simulation in sync.
