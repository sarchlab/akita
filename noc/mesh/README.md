# Rectangular mesh connector

`mesh` grows a rectangular grid from the origin to the largest added coordinate,
including empty transit tiles. Links join neighbors along X, Y, and Z. Use zero
for the third coordinate in a 2D mesh. Negative coordinates are rejected.

```go
connector := mesh.MakeBuilder().WithSimulation(s).
    WithFreq(timing.GHz).WithFlitSize(16).
    WithInputWidth(2).WithOutputWidth(2).
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
connector finalization call.

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
