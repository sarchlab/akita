# NVLink topology connector

`nvlink` assembles PCIe-style attachment trees plus direct device-to-device
connections. All links are ideal. The name describes topology, not a calibrated
NVLink transport or hardware-version model.

```go
connector := nvlink.MakeBuilder().WithSimulation(s).
    WithFlitSize(34).WithPCIeSwitchLatency(140).
    WithNVLinkSwitchLatency(140).Build("Network")
root := connector.AddRootComplex(cpuPorts)
sw := connector.AddSwitch()
connector.ConnectSwitchesWithPCIeLink(root, sw)
a := connector.ConnectDevice(sw, gpu0Ports)
b := connector.ConnectDevice(sw, gpu1Ports)
connector.ConnectDevicesWithNVLink(a, b)
if err := s.Initialize(); err != nil { panic(err) }
```

`ConnectDevice` returns a device ID for `ConnectDevicesWithNVLink`. There is no
lane-count argument. `AddSwitch` creates a PCIe-side switch; connect it explicitly
with `ConnectSwitchesWithPCIeLink`. A device attachment includes its device-side
and NVLink-side internal switches.

Routing uses the fewest-hop `FloydWarshallRouter`. This replaces the previous
bandwidth-first policy whose links all reported infinite bandwidth; chosen
routes and contention timing can change. The generic `networkconnector.Router`
extension remains available for future routing policies.

Defaults are 1 GHz, 34-byte flits, one-flit processing widths, 16-flit port
buffers, and one-flit internal switch queues. PCIe and NVLink switch latency
default to 140 cycles and are independently set with `WithPCIeSwitchLatency`
and `WithNVLinkSwitchLatency`. Internal attachment switches retain their
one-cycle latency. These are simulation parameters, not hardware calibration.

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
