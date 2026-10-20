# PCIe topology connector

`pcie` assembles a tree of ideal-connected switches. It does not implement PCIe
link serialization or the PCIe protocol.

```go
connector := pcie.MakeBuilder().WithSimulation(s).
    WithFreq(timing.GHz).WithFlitSize(34).
    WithInputWidth(1).WithIncomingBufferCapacity(16).
    WithSwitchLatency(140).Build("PCIe")
root := connector.AddRootComplex(cpuPorts)
sw := connector.AddSwitch(root)
connector.ConnectDevice(sw, gpuPorts)
if err := s.Initialize(); err != nil { panic(err) }
```

`AddRootComplex` creates a switch and attaches CPU ports. `AddSwitch(parent)`
creates and connects a child switch. `ConnectDevice(switchID, ports)` attaches a
device's ports through an endpoint. Switch IDs belong to their connector.

Defaults preserve the former effective configuration: 1 GHz, 34-byte flits,
140-cycle switch latency, width one, 16-flit port buffers, and one-flit internal
switch queues. The unusual 34-byte default preserves the prior rounded flit
size; it is not a PCIe hardware claim. `WithSwitchLatency(n)` sets switch
processing latency in cycles and requires a nonnegative value.

## Direct configuration

These connectors model ideal direct links. Physical link bandwidth, propagation
latency, lane count, and hardware-version presets are not modeled. A separate
calculator may translate hardware targets into explicit configurations in a
future release. Switch processing and buffering still constrain throughput.

Builders use value receivers and `Build(name)` creates an independent network.
All assembly must finish before `simulation.Initialize()`. Initialization computes
and validates routes, freezes them, and initializes component state. There is no
connector finalization call. Network wiring, port order, queue settings, and
computed routes participate in checkpoint compatibility checks.

| Builder option | Meaning |
| --- | --- |
| `WithSimulation(s)` | Owning simulation, required |
| `WithFreq(f)` | Component and ideal-connection clock |
| `WithFlitSize(n)` | Bytes per flit, independent of frequency |
| `WithInputWidth(n)` | Flits admitted per input per cycle |
| `WithOutputWidth(n)` | Flits ejected per output per cycle |
| `WithIncomingBufferCapacity(n)` | Incoming port queue capacity in flits |
| `WithOutgoingBufferCapacity(n)` | Outgoing port queue capacity in flits |
| `WithRouteBufferCapacity(n)` | Per-input switch routing queue capacity |
| `WithForwardBufferCapacity(n)` | Per-input switch forwarding queue capacity |
| `WithSendBufferCapacity(n)` | Per-output switch send queue capacity |

Widths and queue capacities must be positive and are independently configurable.
Changing frequency does not change flit size. Increasing a queue's depth does not
increase its processing width. The input/output options also configure endpoint
processing. Device-facing ports remain the ports supplied by the caller; buffer
options configure the endpoint network port and switch ports.
