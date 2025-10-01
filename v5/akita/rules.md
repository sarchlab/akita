# Akita V5 Component Rules

This document defines numbered rules for Akita V5-style components and what the `akita` CLI lints for. 

1. Component
  1.1 A package (directory) defines at most one component.
  1.2 A component package must include a `//akita:component` comment (no space after the slashes) to mark the root.
  1.3 Must define a `type Comp struct { ... }` in `comp.go`.

2. State
  2.1 Must define `state` struct with pure-data. Primitives are always OK. List and maps are OK as long as the values are pure data. Structs can also be considered pure data if the internal fields are all pure data. No pointers to other elements. (No interfaces, channels, functions, etc.)

3. Spec
  3.1 Must define an immutable `Spec` containing only configuration.
  3.2 Must provide `func defaults() Spec` with sane defaults.
  3.3 Must provide `func (s Spec) validate() error` for runtime validation.
  3.4 Must follow the same pure-data rule as `state`.


4. Builder
  4.1 Must define a `type Builder struct { ... }` in `builder.go`.
  4.2 Must include a field `simulation simv5.Simulation`, which is set by `WithSimulation(sim) Builder`.
  4.3 Must include a field `spec Spec`, which is set by `WithSpec(spec) Builder`.
  4.5 Must define `func (b Builder) Build(name string) *Comp`.
  4.6 Must validate the spec in `Build()` before using the spec.
