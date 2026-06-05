# networkconnector — Topology Assembly and Routing

Package `networkconnector` provides a connector that wires switches, endpoints,
and links into an arbitrary network topology for the Akita simulation framework.
Within the `noc` networking stack it is the assembly layer: it builds the
`switches` and `endpoint` components, connects them with links, and computes the
`routing` tables. The higher-level mesh, PCIe, and NVLink builders are thin
wrappers that drive a `Connector` to lay out their specific topologies.

## Key Types

### Connector

`Connector` is a value-style builder that accumulates switches and devices and
then establishes routes. Configuration methods return a copy:

```go
conn := networkconnector.MakeConnector().
    WithEngine(engine).            // or WithRegistrar(sim)
    WithMonitor(monitor).
    WithDefaultFreq(1 * timing.GHz).
    WithFlitSize(64).
    WithRouter(networkconnector.FloydWarshallRouter{})
```

Topology methods:

- `NewNetwork(name)` — start a fresh network.
- `AddSwitch()` / `AddSwitchWithName(name)` — add a switch, returns its ID.
- `ConnectDevice(switchID, ports, param)` — create an endpoint for the device's
  ports and link it to a switch.
- `ConnectSwitches(leftID, rightID, param)` — add a bidirectional switch link.
- `EstablishRoute()` — run the router to populate every switch's routing table.

Link parameters (`DeviceToSwitchLinkParameter`, `SwitchToSwitchLinkParameter`,
and their `LinkEnd*`/`LinkParameter` fields) configure buffer sizes, channel
counts, latency, and link frequency. Only ideal (zero-latency `directconnection`)
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

`ConnectDevice` builds an `endpoint`, creates the switch-side port, adds it to
the switch with a `SwitchPortAdder`, and links the two with a
`directconnection`, recording the link as `Remote`s on both nodes.
`ConnectSwitches` does the same symmetrically for two switches. After the
topology is described, `EstablishRoute` gathers all nodes and lets the chosen
`Router` fill in every switch's `routing.Table` so that flits can reach any
device.
