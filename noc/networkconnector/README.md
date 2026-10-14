# networkconnector — Topology Assembly and Routing

Package `networkconnector` provides a connector that wires switches, endpoints,
and links into an arbitrary network topology for the Akita simulation framework.
Within the `noc` networking stack it is the assembly layer: it builds the
`switches` and `endpoint` components, connects them with links, and computes the
`routing` tables. The higher-level mesh, PCIe, and NVLink builders are thin
wrappers that drive a `Connector` to lay out their specific topologies.

## Key Types

### Connector

`Builder` configures a new `*Connector`. Its configuration methods return a copy;
`Build(name)` allocates independent assembly state. The connector accumulates
switches and devices and then establishes routes:

```go
conn := networkconnector.MakeBuilder().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    WithFlitSize(64).
    WithRouter(networkconnector.FloydWarshallRouter{}).
    Build("Network")
```

Topology methods:

- `AddSwitch()` / `AddSwitchWithName(name)` — add a switch, returns its ID.
- `ConnectDevice(switchID, ports, param)` — create an endpoint for the device's
  ports and link it to a switch.
- `ConnectSwitches(leftID, rightID, param)` — add a bidirectional switch link.
- `EstablishRoute()` — compute routes using planned port names, capture them in
  each switch's Spec, build and bind the switches, and complete pending links. Call it once,
  after the last `ConnectDevice` and `ConnectSwitches`: `Spec.Links` is
  fixed at Build, so each switch is built only when its links are final.
  Call `simulation.Initialize()` after the remaining platform wiring.

Link parameters (`DeviceToSwitchLinkParameter`, `SwitchToSwitchLinkParameter`,
and their `LinkEnd*`/`LinkParameter` fields) configure buffer sizes, channel
counts, latency, and link frequency. Only ideal (zero-latency `direct.Connection`)
links are implemented; non-ideal links panic.

### Node, Remote, Router

`Node` abstracts a switch or endpoint, exposing `ListRemotes()`, `Table()`, and
`Name()`. A `Remote` records one directed link (local/remote node, ports, and
the `messaging.Connection`). A `Router` computes routes over the node list:

```go
type Router interface {
    EstablishRoute(nodes []Node)
}
```

Two implementations are provided: `FloydWarshallRouter` (fewest hops, the
default) and `BandwidthFirstRouter` (maximizes the minimum link bandwidth along
a path). Both run Floyd–Warshall and then call `DefineRoute` on each switch's
table for every reachable device port.

## How It Works

`ConnectDevice` builds an `endpoint`, binds its network port, attaches the
device ports through `endpoint.ConnectDevices`, and creates the switch-side
port. It creates a `direct.Connection` and records the pending attachment
and `Remote`s on both nodes. `ConnectSwitches` similarly records a pending
link between two switches. Because `Spec.Links` is fixed at Build, the
connector waits until the topology is described before building switches in
`EstablishRoute`. Its `BuildSwitches` step binds every switch port and then
attaches both ends of each pending connection. The chosen `Router` first fills each assembly-time routing table; a sorted
snapshot becomes `Spec.Routes` and `Spec.DefaultRoute`. Switch middleware derives
its private lookup table from that Spec during initialization. Adding a port to
a switch that is already built panics. Initialize the simulation after all
platform wiring is complete.

## Supported ports and links

The connector creates `twowaybuffered.Port` instances directly and uses only
`direct.Connection` links. There is no configurable port factory. Device ports
must be `*twowaybuffered.Port`; endpoint construction rejects other implementations
before attaching device ports. Component port fields remain `messaging.Port`.
Wire ports and non-ideal links are outside this connector's current support.
