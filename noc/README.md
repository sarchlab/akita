# noc

Package `noc` provides packet-switched networks with endpoints, switches, and
routing. Builders currently use only `twowaybuffered.Port` and ideal
`direct.Connection` links. Device ports must also be `*twowaybuffered.Port`.
Wire ports and links with finite bandwidth or latency are not supported yet.

## Direct Connection

The simplest connection type — zero-latency message forwarding between ports.

```go
import (
    "github.com/sarchlab/akita/v5/sim/messaging/direct"
    "github.com/sarchlab/akita/v5/sim/timing"
)

conn := direct.NewConnection("conn", sim, timing.GHz)

conn.PlugIn(componentA.Ports.Top)
conn.PlugIn(componentB.Ports.Top)
```

`direct.Connection` lives in `sim/messaging/direct` and is a connection that forwards messages
between all plugged-in ports each tick, with no latency or bandwidth
modeling.

## Networking (Packet-Switched Networks)

The `noc/networking` sub-packages provide realistic network models with
switches, endpoints, routing, and switch pipelines. Links between them are ideal.

### Network Connector

`networkconnector.Connector` builds complex topologies by connecting devices
and switches with parameterized links:

```go
import "github.com/sarchlab/akita/v5/noc/networking/networkconnector"

connector := networkconnector.MakeConnector().
    WithSimulation(sim).
    WithDefaultFreq(1 * timing.GHz).
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

Package `noc/packetization` defines the `Flit` type — the smallest transfer
unit on a network:

```go
type Flit struct {
    SeqID        int               // flit sequence number within a message
    NumFlitInMsg int               // total flits in the parent message
    Msg          messaging.Msg // metadata of the carried message
}
```

Endpoints fragment `messaging.Msg` into `Flit`s for transmission and reassemble
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

Flits are registered payload values inside `messaging.Msg`. Only flit 0 carries the complete
original message; every flit has a small routing and reassembly header. The
receiving endpoint delivers the original metadata and typed application payload
after all flits arrive, including after checkpoint restoration.
