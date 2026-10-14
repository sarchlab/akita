# Akita v5 architecture rules

These rules record the decisions in [issue #546](https://github.com/sarchlab/akita/issues/546).

## Simulation lifecycle

Use `sim.MakeBuilder().Build()` for tests, examples, and full simulations. The
simulation owns the engine, IDs, inventory, and initialization. Recording is off
by default. `WithOutputPath(path)`, `WithVisTracingOnStart`, or `WithMonitor`
explicitly enables recording; a supplied path is used exactly and existing files
are rejected. `WithTaskTracking` alone works in memory and leaves `VisTracer` nil.
Custom engine injection is deferred to v5.2.

Build components, bind ports/connections, then call `Initialize()`. Initialization
validates the complete setup before creating State and Middlewares and freezes
topology. Configuration-only checks belong at Build; checks requiring bound ports
belong in initialization. The name remains Initialize because it also constructs
runtime state. `Simulation.Run()` checks readiness. Direct `Engine.Run()` and
`SerialEngine.RunUntil()` document initialization as a caller precondition and
do not check it. RunUntil remains a concrete serial-engine API for deterministic
checkpoint/testing boundaries, not part of Engine or Simulation.

## Component ownership

- Spec holds immutable configuration, including destination addresses and routing
  parameters. It accepts scalars, scalar arrays/slices, and slices of flat records.
  Each record field must be an exported scalar with an explicit, unique JSON name.
  No nested containers, maps, pointers, interfaces, or standalone nested structs.
- State holds mutable runtime data and is checkpointed. Mutable state must not
  hide on a middleware.
- Resources holds shared live objects such as storage and page tables. It must
  not duplicate Spec routing through injected mappers or destination lists.
- Ports holds bound local ports. `Spec.Links[i]` describes `Ports.Links[i]`.
  Build checks link configuration; Initialize checks the length equality.
- Middlewares implement behavior and derive private lookup tables/mappers from
  Spec. Spec participates in checkpoint compatibility checks.

Runtime validation, static inspection, literal default extraction, and schema
output must accept the same Spec shapes. Definition.ValidateSpec checks values at
Build; Definition.ValidateSetup checks binding-dependent invariants at Initialize
before any State or Middlewares is created.

## Builders and packages

With methods belong only to value Builders and return an updated Builder. A Build
call returns a fresh object. Working objects use explicit action methods rather
than With methods. Use WithFreq consistently. Derived parameters are computed at
Build so frequency/bandwidth option order does not matter.

Monitor setup is `monitoring.MakeBuilder().WithPortNumber(port).Build()`.
The networkconnector, mesh, pcie, and nvlink packages use
`MakeBuilder().With...().Build(name) *Connector`; assembly methods operate on that
connector. Reusing a builder creates independent networks without reset methods.

NoC packages live directly below `noc`, without a `networking` layer. The cache
package is `mem/cache/writethrough`; it retains write-around, write-evict, and
write-through policies. Group packages by responsibility and avoid redundant
directory/package suffixes.
