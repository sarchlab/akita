# Rectangular mesh connector

`mesh` grows a rectangular grid from the origin to the largest added coordinate,
including empty transit tiles. Links join neighbors along X, Y, and Z. Use zero
for the third coordinate in a 2D mesh. Negative coordinates are rejected.

```go
connector := mesh.MakeBuilder().WithSimulation(s).
    WithFreq(timing.GHz).WithFlitSize(16).
    WithInputWidth(2).WithOutputWidth(2).
    WithIncomingBufferCapacity(8).WithOutgoingBufferCapacity(8).
    WithSwitchLatency(1).Build("Mesh")
connector.AddTile([3]int{0, 0, 0}, tile0Ports)
connector.AddTile([3]int{1, 0, 0}, tile1Ports)
if err := s.Initialize(); err != nil { panic(err) }
```

Repeated `AddTile` calls append device ports to the existing endpoint in binding
order. Routing is Z-then-Y-then-X dimension order. Arbitration follows port
binding order, so changing assembly order can change contention timing.

Defaults are 1 GHz, 16-byte flits, widths one, one-flit port and internal switch
buffers, zero-cycle inter-switch processing latency, and one-cycle processing
on device-facing switch ports. `WithSwitchLatency` changes the inter-switch
processing latency; it must be nonnegative.

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
