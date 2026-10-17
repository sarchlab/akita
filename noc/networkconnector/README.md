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
- `ConnectDeviceWithEPName(name, switchID, ports, param)` — create a named
  endpoint, or append ports to it when the switch and parameters match.

Call `simulation.Initialize()` after all platform wiring. There is no separate
connector finalization call, address prediction, or returned internal switch port.

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

`AddSwitch` builds a switch immediately. Connect calls bind both port owners and
attach both ends of the connection. Indexed port groups grow until Initialize.
The network's shared topology resource computes routes during setup validation
from actual bound addresses. It returns errors for disconnected networks, invalid
next hops, and forwarding loops before any component initializes runtime state.
Accessing wiring during assembly does not compute or cache routes.

Custom `Router` implementations use the concrete `routing.Table` type. Changing
tables after Initialize is not supported. Switch arbitration uses port binding
order; the routing interface does not configure arbitration priority.

`ConnectDeviceWithEPName` may be called repeatedly with the same endpoint name,
switch, and parameters. Each call appends device ports in call order to the existing
endpoint. Reusing the name with a different switch or parameters is rejected.

One network topology record validates wiring, port/device order, link settings,
and computed routes on checkpoint restore, without depending on Go policy type
names. Endpoint peers are explicitly supplied through Resources. Direct networks
built without a connector register their own topology record (see `one_to_one`).
Runtime route mutation and non-ideal link transport are deferred.

## Supported ports and links

The connector creates `twowaybuffered.Port` instances directly and uses only
`direct.Connection` links. There is no configurable port factory. Device ports
must be `*twowaybuffered.Port`; endpoint construction rejects other implementations
before attaching device ports. Component port fields remain `messaging.Port`.
Wire ports and non-ideal links are outside this connector's current support.
