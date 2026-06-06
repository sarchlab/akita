# mesh — 2D/3D Mesh and Torus Network-on-Chip

Package `mesh` provides a connector that builds a mesh (or torus) network
topology for the Akita simulation framework. Within `noc`, it constructs the
switches, links, and routing tables that move messages between components laid
out on a 2D or 3D grid of tiles.

## How It Works

A `Connector` lays out tiles on a grid of up to three dimensions. Each tile
holds one or more device ports and is backed by a switch. When the network is
established, the connector:

1. Creates one switch per occupied tile and connects each tile's device ports
   to its switch.
2. Wires neighboring switches together with bidirectional links along the
   left/right (X), top/bottom (Y), and front/back (Z) directions.
3. Installs a `meshRoutingTable` on every switch that performs
   dimension-ordered (Z, then Y, then X) routing toward the destination tile.

Links are modeled with the configured frequency, flit size, per-link bandwidth
(transfers per cycle), and switch latency. 2D meshes are built by leaving the
third coordinate at `0`.

## Key Types

### Connector

```go
type Connector struct { /* ... */ }

func NewConnector() *Connector
func (c *Connector) CreateNetwork(name string)
func (c *Connector) AddTile(loc [3]int, ports []messaging.Port)
func (c *Connector) EstablishNetwork()
```

`AddTile` places device ports at grid coordinate `loc` (negative coordinates
are rejected; calling it twice for the same coordinate merges the ports). The
grid grows automatically beyond its default `8x8x2` capacity as tiles are
added. `EstablishNetwork` then creates the switches, links, and routing tables.

The internal `meshRoutingTable` (in `mesh_routing_table.go`) resolves each
destination port to its tile and returns the next-hop port.

## Builder Pattern

`NewConnector` returns a `Connector` configured with chained `WithX` options:

```go
connector := mesh.NewConnector().
    WithEngine(engine).
    WithFreq(1 * timing.GHz).
    WithFlitSize(16).
    WithBandwidth(1).      // transfers per cycle, per link
    WithSwitchLatency(1).  // cycles per switch hop
    WithMonitor(monitor).
    WithVisTracer(visTracer).
    WithNoCTracer(nocTracer)
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFreq(f)` | Frequency the network runs at |
| `WithFlitSize(n)` | Flit size in bytes |
| `WithBandwidth(t)` | Per-link bandwidth as transfers per cycle |
| `WithSwitchLatency(n)` | Latency in cycles added at each switch |
| `WithMonitor(m)` | Monitor for inspecting component state |
| `WithVisTracer(t)` | Tracer for visualizing network tasks |
| `WithNoCTracer(t)` | Tracer for NoC traffic and congestion metrics |

## Usage

```go
connector := mesh.NewConnector().
    WithEngine(engine).
    WithFreq(1 * timing.GHz)

connector.CreateNetwork("Mesh")

connector.AddTile([3]int{0, 0, 0}, []messaging.Port{tile00.GetPortByName("Net")})
connector.AddTile([3]int{1, 0, 0}, []messaging.Port{tile10.GetPortByName("Net")})
connector.AddTile([3]int{0, 1, 0}, []messaging.Port{tile01.GetPortByName("Net")})

connector.EstablishNetwork()
```

Once established, a message sent from one tile's port with its `Dst` set to a
port on another tile is routed hop-by-hop across the mesh to its destination.
