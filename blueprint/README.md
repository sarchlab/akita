# Declarative simulation assembly

A Blueprint describes a simulation before its components exist. A domain is a
reusable declaration function: it only adds nodes, connections, and exposures.
`Simulation.Initialize` expands the domains, resolves references, creates shared
resources and components, binds ports and connections, assembles connector-owned
networks, and runs the existing setup validation and initialization.

```go
s := sim.MakeBuilder().Build()
s.Add("", platform.Node(platform.DefaultSpec()))
if err := s.Initialize(); err != nil {
    panic(err)
}
agent := blueprint.Lookup[*agent.Comp](s, "Agent")
// Seed workload state only after initialization, then run.
```

## Components, resources, and domains

`blueprint.Comp[*cache.Comp](spec)` retains the existing model builder's typed
construction capability without invoking it. Configure every local port with
`.Port("Top", twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 8})`.
Both capacities are required and positive. Groups use indexed slots such as
`Links[0]`, with contiguous indices. No port buffer sizes are inferred.

`blueprint.Res(mem.StorageSpec{Capacity: 1024, UnitSize: 64})` declares a shared
resource. `Blueprint.Add` returns its absolute path. `.Use("Storage", path)`
links a component's Resources field to that declaration. Resource paths are global
absolute paths; port exposure boundaries do not restrict resource sharing. Resource dependencies
use the same mechanism; they are type-checked and built before their consumers.
Dependency cycles are errors. To declare a custom resource, its Spec implements
`BuildResource(modeling.Simulation, string, Resources) *Resource`; use
`modeling.None` for an empty Resources struct. That method creates and registers
one named resource only when initialization invokes it.

A domain package declares `Spec`, `DefaultSpec() Spec`, a static `Ports` struct,
and the constructor:

```go
func Node(spec Spec) *blueprint.DomainNode {
    return blueprint.NewDomain[Spec, Ports](spec, build)
}
```

`build(spec, bp)` adds nodes. Each Add name is one path segment (for example,
`Child[3]`); dotted paths are only used for references, not node names. A nested `bp.Add("Child", child.Node(childSpec))`
retains an unexpanded child. Specs obey the same shape rules as component Specs;
configuration slices are copied, including slices within flat records. Domain
Ports contain only `messaging.Port` and `[]messaging.Port` fields. Each single
port must be exposed; every group element must be exposed at contiguous indices
from zero. Empty groups are allowed. `bp.Expose("Top", "Cache.Top")` maps a public
slot to an inner relative path. Domains have no runtime State; after initialization,
`blueprint.Lookup[domain.Ports](s, "Domain")` returns their actual exposed ports.
The source inspector discovers these declarations without running domain code.

## References and ordering

`bp.PortRef("Child.Top")` returns a port address relative to the current domain,
converted to an absolute address. It can reference a later declaration. A parent
passes these already absolute values through child Specs unchanged; do not call
`PortRef` again on them. Across domain boundaries, target exposed ports rather than
private inner component paths. Nested exposures resolve before component Build.
The root is the current domain's declaration environment, not the filesystem or
the whole simulation: inside `Platform.D`, `bp.PortRef("D.Top")` means
`Platform.D.D.Top`. There is no prefix guessing. `Add` returns an already absolute
node path; do not feed that return value back into `PortRef`.

For example, a parent passes `child.Spec{Peer: bp.PortRef("Memory.Top")}` to
`bp.Add("Child", child.Node(spec))`. The child's declaration uses `spec.Peer`
directly in its component Spec. It uses its own `bp.PortRef("Cache.Top")` only
for ports relative to that child. The same declaration works under any parent
name, and references to exposed domain ports resolve to the underlying component
ports before their builders run. No runtime port IDs or handles are introduced.

Only fields typed `messaging.RemotePort` are resolved; ordinary strings are data.
Empty optional fields remain empty, with requiredness checked by the component's
own validation. `messaging.Unconnected` marks an intentionally absent destination;
it is accepted in configuration but sending to it panics with the sender/address.
It cannot be attached as a connection member.

`bp.Connect("Bus", freq, bp.PortRef("A.Top"), bp.PortRef("B.Top"))` declares a direct
connection. Member order is binding/arbitration order. Node declarations and
resource dependencies are processed deterministically without map-order behavior.
Forward-reference support does not promise that arbitrary changes in arbitration
or scheduling order preserve timing.

`blueprint.Expand(bp)` returns an independent flat blueprint and an error. It
creates no runtime objects. `Nodes()` returns detached copies for inspection.
Invalid names, missing buffers, bad references, incomplete exposures, resource
mismatches and cycles include the node path and field in their errors. Recursive
domain expansion is limited to 256 levels to diagnose unbounded recursion.

## Networks and custom connectors

`mesh.Node(spec)`, `pcie.Node(spec)`, and `nvlink.Node(spec)` declare networks.
Their Specs describe settings, ordered attachments, and topology. `Expand` retains
these declarations; it does not list internal switches or endpoints. The existing
connectors create those objects after the ordinary device ports exist, before
setup validation computes routes.

Custom connectors use `blueprint.NewNetwork(spec, members, assemble)`. The typed
callback receives `(spec, simulation, name, []messaging.Port)`: the framework
resolves members to actual ports in declaration order before invoking it. The
callback creates and wires the network through the normal builders. It must not
capture live objects or perform construction when the declaration is created.
Specs may define `Validate() error` for checks during Expand. Network declarations
snapshot their settings and references, and expose detached read copies through
`Configuration` and `PortReferences` so these cannot drift through later mutation.

## Failure and compatibility

Blueprint failures during `Simulation.Initialize` panic. Discard that Simulation;
there is no rollback or supported retry after recovery. Calling Initialize twice
or adding nodes after initialization has been attempted also panics. `Initialize() error` keeps
its existing signature, and an empty blueprint preserves the hand-built path.
Manual and Blueprint assembly are separate supported paradigms; mixing them is
outside the contract, without a dedicated prohibition mechanism.

Checkpoints still save runtime State, not domains or blueprint configuration.
The same executable and platform must restore exactly; cross-executable restore
is best effort, without versions or configuration hashes. JSON source generation
is tracked separately in issue #594 and is not a runtime Blueprint loader.
