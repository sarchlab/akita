# noc

Package `noc` and its sub-packages provide network-on-chip (NoC)
implementations for connecting simulation components. All network
types implement the `sim.Connection` interface, using `PlugIn(port)`
to attach component ports.

## Direct Connection

The simplest connection type — zero-latency message forwarding between ports.

```go
import "github.com/sarchlab/akita/v5/noc/directconnection"

conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("conn")

conn.PlugIn(componentA.GetPortByName("Top"))
conn.PlugIn(componentB.GetPortByName("Top"))
```

`directconnection.Comp` is a `TickingComponent` that forwards messages
between all plugged-in ports each tick, with no latency or bandwidth
modeling.

## Networking (Packet-Switched Networks)

The `noc/networking` sub-packages provide realistic network models with
switches, endpoints, routing, and pipeline-based links.

### Network Connector

`networkconnector.Connector` builds complex topologies by connecting devices
and switches with parameterized links:

```go
import "github.com/sarchlab/akita/v5/noc/networking/networkconnector"

connector := networkconnector.NewConnector().
    WithEngine(engine).
    WithDefaultFreq(1 * sim.GHz).
    WithFlitSize(64)

// Add switches, connect devices, build routing tables...
```

### Topology Packages

| Package | Description |
|---------|-------------|
| `networking/mesh` | 2D mesh topology builder and routing tables |
| `networking/pcie` | PCIe topology connector |
| `networking/nvlink` | NVLink topology connector |

### Infrastructure Packages

| Package | Description |
|---------|-------------|
| `networking/switching/switches` | Switch components with input/output buffers and crossbar |
| `networking/switching/endpoint` | Network endpoints that fragment messages into flits |
| `networking/routing` | Routing table abstraction |
| `networking/networkconnector` | Generic topology builder with Floyd-Warshall routing |

## Messaging

Package `noc/messaging` defines the `Flit` type — the smallest transfer
unit on a network:

```go
type Flit struct {
    sim.MsgMeta
    SeqID        int         // flit sequence number within a message
    NumFlitInMsg int         // total flits in the parent message
    Msg          sim.MsgMeta // metadata of the carried message
    OutputBufIdx int         // output buffer index within a switch
}
```

Endpoints fragment `sim.Msg` into `Flit`s for transmission and reassemble
them at the destination.

## Connection Pattern

All connection types follow the same pattern:

```go
// 1. Build the connection
conn := builder.Build("my_connection")

// 2. Plug in ports
conn.PlugIn(portA)
conn.PlugIn(portB)

// 3. Components send via their ports as usual
portA.Send(msg) // connection handles delivery to portB
```

Components are unaware of the underlying network topology — they simply
send messages through their ports.
