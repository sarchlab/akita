# PCIe topology connector

`pcie` assembles a tree of ideal-connected switches. It does not implement PCIe
link serialization or the PCIe protocol.

```go
connector := pcie.MakeBuilder().WithSimulation(s).
    WithFreq(timing.GHz).WithFlitSize(34).
    WithInputWidth(1).WithOutputWidth(1).
    WithSwitchLatency(140).Build("PCIe")
root := connector.AddRootComplex(cpuPorts)
sw := connector.AddSwitch(root)
connector.ConnectDevice(sw, gpuPorts)
if err := s.Initialize(); err != nil { panic(err) }
```

`AddRootComplex` creates a switch and attaches CPU ports. `AddSwitch(parent)`
creates and connects a child switch. `ConnectDevice(switchID, ports)` attaches a
device's ports through an endpoint. Switch IDs belong to their connector.

Defaults are 1 GHz, 34-byte flits, 140-cycle switch latency, width one,
and one-flit network-owned port and internal switch queues. Port buffers were
previously 16 flits; the smaller queues can change backpressure and timing. The unusual 34-byte default preserves the prior rounded flit
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

Input/output widths must be positive and are configured separately. Changing
frequency does not change flit size. Network-owned buffers are calculated:
incoming port, route, and forward queues hold `InputWidth` flits; send and
outgoing port queues hold `OutputWidth` flits. Endpoint network ports use the
same rule. Device-facing ports remain caller-owned with their original sizes.
Forwarding shares each output's width among contenders, limits each input to
its input width, and rotates priority in binding order each cycle. Buffering is
one cycle of capacity, not a guarantee of full throughput under contention.
