# pcie — PCIe Root-Complex and Switch Interconnect

Package `pcie` provides a connector that builds a PCIe-style interconnect for
the Akita simulation framework. Within `noc`, it constructs a tree of switches
rooted at a root complex and moves messages between the CPU and attached
devices over links sized to a chosen PCIe version.

## How It Works

A `Connector` builds the network from switches. One switch acts as the root
complex (attached to the CPU ports); additional switches branch off existing
switches to form a tree. Device ports plug into any switch.

Link bandwidth is derived from the configured PCIe version and lane width: the
per-lane transfer rate is multiplied by the width and divided by 8 to get
bytes/second, which together with the network frequency determines the flit
size (`bandwidth / freq`). Each switch adds a configurable forwarding latency.
After the topology is built, routing tables are populated in a single pass.

## Key Types

### Connector

```go
type Connector struct { /* ... */ }

func NewConnector() *Connector
func (c *Connector) CreateNetwork(name string)
func (c *Connector) AddRootComplex(cpuPorts []messaging.Port) (switchID int)
func (c *Connector) AddSwitch(baseSwitchID int) (switchID int)
func (c *Connector) PlugInDevice(baseSwitchID int, devicePorts []messaging.Port)
func (c *Connector) EstablishRoute()
```

`AddRootComplex` creates the root switch and connects the CPU ports to it,
returning its switch ID. `AddSwitch` adds a downstream switch linked to an
existing one. `PlugInDevice` attaches device ports to a switch. Switch IDs
returned by these calls are used to wire the tree together.

## Builder Pattern

`NewConnector` returns a `Connector` configured with chained `WithX` options.
By default it runs at `1 GHz`, PCIe version 4 at width 16, and a switch latency
of 140 cycles.

```go
connector := pcie.NewConnector().
    WithEngine(engine).
    WithFrequency(1 * timing.GHz).
    WithVersion(4, 16).      // PCIe Gen4, x16 lanes
    WithSwitchLatency(140).  // cycles per switch hop
    WithMonitor(monitor).
    WithVisTracer(visTracer)
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFrequency(f)` | Frequency of the network interfaces |
| `WithVersion(v, w)` | PCIe generation (1–5) and lane width; sets bandwidth |
| `WithBandwidth(b)` | Set link bandwidth directly in bytes/second |
| `WithSwitchLatency(n)` | Latency in cycles added at each switch |
| `WithMonitor(m)` | Monitor for inspecting component state |
| `WithVisTracer(t)` | Tracer for visualizing the network |

## Usage

```go
connector := pcie.NewConnector().
    WithEngine(engine).
    WithFrequency(1 * timing.GHz).
    WithVersion(4, 16)

connector.CreateNetwork("PCIe")

root := connector.AddRootComplex([]messaging.Port{cpu.GetPortByName("PCIe")})
sw := connector.AddSwitch(root)
connector.PlugInDevice(sw, []messaging.Port{gpu.GetPortByName("PCIe")})

connector.EstablishRoute()
```

After `EstablishRoute`, a message sent from the CPU port to a device port is
routed down the switch tree to its destination, accumulating switch and link
latency along the way.
