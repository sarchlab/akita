# Akita v5 architecture rules

These rules record the decisions in [issue #546](https://github.com/sarchlab/akita/issues/546).

## Simulation lifecycle

Use `sim.MakeBuilder().Build()` for tests, examples, and full simulations. The
simulation owns the engine, IDs, inventory, and initialization. Recording is off
by default. `WithOutputPath(path)`, `WithVisTracingOnStart`, or `WithMonitor`
explicitly enables recording; a supplied path is used exactly and existing files
are rejected. `WithTaskTracking` alone works in memory and leaves `VisTracer` nil.
Custom engine injection is deferred to v5.2.

Build components, bind ports/connections, then call `Initialize()`. Build checks
structural Spec/State requirements and basic model prerequisites. Initialize
checks generic port bindings before freezing topology, then invokes each
component's optional State.Initialize and Middlewares.Initialize methods, in
that order. Component-specific checks belong in the relevant initializer;
a panic leaves the simulation unusable and requires rebuilding. Generic binding
errors returned before initialization leave setup editable. The name remains
Initialize because it also constructs runtime state. `Simulation.Run()` checks readiness. Direct `Engine.Run()` and
`SerialEngine.RunUntil()` document initialization as a caller precondition and
do not check it. RunUntil remains a concrete serial-engine API for deterministic
checkpoint/testing boundaries, not part of Engine or Simulation.

## Component ownership

- Spec holds immutable configuration, including destination addresses and routing
  parameters. It accepts scalars, scalar arrays/slices, flat record fields, and
  slices of flat records. A record may contain exported scalars or scalar slices
  with explicit, unique JSON names. Records cannot nest; maps, pointers,
  interfaces, and other nested containers are prohibited. Build copies all
  slices, including those within records.
- State holds mutable runtime data and is checkpointed. Mutable state must not
  hide on a middleware.
- Resources holds shared live objects such as storage and page tables. Public
  components must not duplicate Spec routing through injected mappers or destination
  lists. Internal NoC components reference completed network wiring: routes and
  peers are derived at Initialize; changing routing tables afterwards is not supported.
- Ports holds bound local ports. Internal NoC port groups grow during setup.
  Their link-end settings belong to network assembly and are checked at Initialize;
  switch pipeline latency and admission/ejection widths retain their per-port meaning.
- Middlewares implement behavior and may derive private lookup tables from Spec.
  Memory routing uses `Spec.Bottom.Find(address)` (or the corresponding port
  name) directly. Spec participates in checkpoint compatibility checks.

Expose default configurations and presets as functions that return an independent
value on each call. Do not store defaults or presets in exported mutable package
variables. Any mutable collections in a returned configuration must be independent
across calls.

Runtime validation, static inspection, literal default extraction, and schema
output must accept the same Spec shapes. A package declares a Comp alias for
its component model and func DefaultSpec() Spec, which directly returns a keyed
Spec literal with constant leaves. The inspector reads those declarations without
executing code; there is no component Definition variable or type.
Build with `modeling.MakeBuilder[*Comp](DefaultSpec())`. The caller supplies the
Spec explicitly; a package-level function is never discovered at runtime.
State and Middlewares may each define a pointer-receiver Initialize(*Comp)
method with no return value. Omission preserves the zero value; middleware
fields must still be valid, non-nil modeling.Middleware values after initialization.
There are no validator or creator registration callbacks. Spec and Resources are
supplied through the builder; Ports are bound by the system builder. Initializers
must not change Spec, Resources, Ports, or another component's State.

## Builders and packages

With methods belong only to value Builders and return an updated Builder. A Build
call returns a fresh object. Working objects use explicit action methods rather
than With methods. Use WithFreq consistently. Derived parameters are computed at
Build so frequency/bandwidth option order does not matter.

Monitor setup is `monitoring.MakeBuilder().WithPortNumber(port).Build()`.
Attach through `sim.Builder.WithMonitor`; Start runs only after successful
Initialize, and Stop precedes recording teardown. The exported `sim.Monitor`
contract contains Start/Stop only. Monitors discover copied component and port
inventory slices; callers retain the monitor they supplied.

Use one `messaging.Port` contract (also aliased by `sim.Port`) for components,
connections, and setup. PortOwner includes Name and BindPort; Connection keeps
BindPort, receives the source in NotifySend, and does not require Hookable.
Custom ports are allowed with compatible connections; NoC accepts only
`*twowaybuffered.Port`. Read-only IncomingBuf/OutgoingBuf views expose Size and
Capacity, returning nil only for an absent buffer. Hooks and milestones stay on
Port. Wire remains deferred under #496.

Page tables are concrete `*vm.PageTable` resources. MMU/GMMU validate page size
at Initialize. Memory payload consumers switch on concrete read/write requests;
there is no AccessReq/AccessRsp abstraction. Component models retain custom hooks
but do not export a shared modeling.Component interface.
The networkconnector, mesh, pcie, and nvlink packages use
`MakeBuilder().With...().Build(name) *Connector`; assembly methods operate on that
connector. Reusing a builder creates independent networks without reset methods.

NoCs are assembled through the public `networkconnector`, `mesh`, `pcie`, and
`nvlink` connectors. Endpoint and switch implementation packages live under
`noc/internal`. Endpoints remain pass-through device connections: their bound device order
sets arbitration order. At Initialize, the endpoint reads the bound address of
the peer port supplied through Resources. No device-facing buffer or link hop is added. Connect calls finish wiring; no connector finalization call is needed.
Routes are computed from actual bound ports in network setup validation, which
returns errors for unreachable destinations and routing loops before component
initialization. Custom setup routers remain supported; `routing.Table` is a
concrete type. Changing tables after Initialize is not supported.
One topology resource per network fingerprints wiring, port/device binding order,
link-end settings, link parameters and computed routes for checkpoint compatibility.
Type names are not part of this record. These fingerprints replace routing fields
in internal NoC Specs; rebuild old checkpoints. Networks assembled directly without
a connector must register their own topology record. Switch arbitration follows
binding order. Runtime route changes require a future checkpointed-state design.
Non-ideal transport and bandwidth-unit unification
remain #502/#542 follow-ups; physical transport latency is distinct from switch
pipeline latency. Other NoC packages live directly below `noc`, without a
`networking` layer. The cache
package is `mem/cache/writethrough`; it retains write-around, write-evict, and
write-through policies. Group packages by responsibility and avoid redundant
directory/package suffixes.
