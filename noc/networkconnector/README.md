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
The network's shared topology resource computes routes once from actual bound
addresses. It validates connectivity, every destination's next hop, and complete
forwarding paths (including custom-router loops). Initializers consume the same
cached result without reading another component's State.

The network copies setup tables into immutable runtime lookups. Custom `Router`
and `routing.Table` implementations remain supported, but later edits to their
tables do not change runtime routing. A router may implement
`OrderPorts(Node) []messaging.Port`, returning every local port exactly once, to
choose deterministic arbitration order; mesh uses this to retain its prior order
while growing incrementally. That order is frozen at Initialize as well.

Topology checkpoint records validate wiring, port/device order, link settings,
routing policy type and computed routes. Endpoint records additionally cover
device order and the bound network peer, including direct endpoint-to-endpoint
connections. Runtime route mutation and non-ideal link transport are deferred.

## Supported ports and links

The connector creates `twowaybuffered.Port` instances directly and uses only
`direct.Connection` links. There is no configurable port factory. Device ports
must be `*twowaybuffered.Port`; endpoint construction rejects other implementations
before attaching device ports. Component port fields remain `messaging.Port`.
Wire ports and non-ideal links are outside this connector's current support.
