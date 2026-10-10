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
    WithSimulation(sim).
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
- `EstablishRoute()` — build and bind the switches, complete pending links,
  then run the router to populate every switch's routing table. Call it once,
  after the last `ConnectDevice` and `ConnectSwitches`: `Resources.Links` is
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
link between two switches. Because `Resources.Links` is fixed at Build, the
connector waits until the topology is described before building switches in
`EstablishRoute`. Its `BuildSwitches` step binds every switch port and then
attaches both ends of each pending connection. The chosen `Router` fills in
every switch's `routing.Table` so flits can reach any device. Adding a port to
a switch that is already built panics. Initialize the simulation after all
platform wiring is complete.

## Supported ports and links

The connector creates `twowaybuffered.Port` instances directly and uses only
`direct.Connection` links. There is no configurable port factory. Device ports
must be `*twowaybuffered.Port`; endpoint construction rejects other implementations
before attaching device ports. Component port fields remain `messaging.Port`.
Wire ports and non-ideal links are outside this connector's current support.
