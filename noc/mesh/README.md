# mesh — 2D/3D Mesh Network-on-Chip

Package `mesh` provides a connector that builds a rectangular mesh network
topology for the Akita simulation framework. Within `noc`, it constructs the
switches, links, and routing tables that move messages between components laid
out on a 2D or 3D grid of tiles.

## How It Works

A `Connector` grows a rectangular grid from the origin to the largest added
coordinates, including empty transit tiles. Each AddTile immediately creates new
switches/endpoints and wires neighboring switches along X, Y and Z. Repeated
calls append devices to the same tile. `simulation.Initialize()` derives Z-then-Y-
then-X dimension-order routing from the finished bindings. There is no separate
finalization call. Runtime arbitration follows deterministic port binding order;
contention timing can differ from v4 and between assembly orders.
Internal port indices also reflect assembly order.

The configured frequency, flit size, and switch latency affect the network
components. Links currently use ideal direct connections at the network's
default frequency: the per-link frequency computed by `WithBandwidth` is
ignored. `WithBandwidth` still sets link-end buffer sizes and channel counts
using the rounded-up transfers per cycle, so it can affect buffering and
congestion, but it does not set the ideal-link frequency. Non-ideal link timing
is not implemented; see
[#502](https://github.com/sarchlab/akita/issues/502). 2D meshes are built by
leaving the third coordinate at `0`.

## Key Types

### Connector

```go
type Connector struct { /* ... */ }

func MakeBuilder() Builder
func (b Builder) Build(name string) *Connector
func (c *Connector) AddTile(loc [3]int, ports []messaging.Port)
```

`AddTile` places device ports at grid coordinate `loc` (negative coordinates
are rejected; calling it twice for the same coordinate merges the ports). The
grid grows to include the requested coordinates during that call. All wiring
must finish before Initialize; later AddTile calls are rejected.

## Builder Pattern

`MakeBuilder` returns a value builder configured with chained `WithX` options:

```go
connector := mesh.MakeBuilder().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    WithFlitSize(16).
    WithSwitchLatency(1). // cycles per switch hop
    Build("Mesh")
```

### Builder Options

| Method | Description |
|---|---|
| `WithSimulation(sim)` | Simulation context (required) |
| `WithFreq(f)` | Frequency the network runs at |
| `WithFlitSize(n)` | Flit size in bytes |
| `WithBandwidth(t)` | Sets link-end buffer sizes and channel counts; does not change ideal-link frequency |
| `WithSwitchLatency(n)` | Latency in cycles added at each switch |

The connector builds its switches and endpoints with `WithSimulation(sim)`,
so the simulation's tracer and monitor see each of them.

## Usage

```go
connector := mesh.MakeBuilder().
    WithSimulation(sim).
    WithFreq(1 * timing.GHz).
    Build("Mesh")

connector.AddTile([3]int{0, 0, 0}, []messaging.Port{tile00.Ports.Net})
connector.AddTile([3]int{1, 0, 0}, []messaging.Port{tile10.Ports.Net})
connector.AddTile([3]int{0, 1, 0}, []messaging.Port{tile01.Ports.Net})

if err := sim.Initialize(); err != nil { panic(err) }
```

Once established, a message sent from one tile's port with its `Dst` set to a
port on another tile is routed hop-by-hop across the mesh to its destination.

All `WithX` methods belong to the value `Builder`. `Build(name)` returns a fresh
`*Connector`; a builder can be reused for independent networks. Derived values
are computed by `Build`, so setting frequency and bandwidth in either order
produces the same configuration. Use the connector's assembly methods after
Build, then call `simulation.Initialize()` after all wiring is complete.
