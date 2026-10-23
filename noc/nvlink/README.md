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
routes and contention timing can change. Devices sharing a PCIe switch take
the PCIe path (two switch-to-switch hops), even when a direct NVLink path
exists (three hops through the NVLink-side switches). The generic `networkconnector.Router`
extension remains available for future routing policies.

Defaults are 1 GHz, 34-byte flits, one-flit processing widths, and one-flit
network-owned port and internal switch queues. Port buffers were previously
16 flits; the smaller queues can change backpressure and timing. PCIe and NVLink switch latency
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

Input/output widths must be positive and are configured separately. Changing
frequency does not change flit size. Network-owned buffers are calculated:
incoming port, route, and forward queues hold `InputWidth` flits; send and
outgoing port queues hold `OutputWidth` flits. Endpoint network ports use the
same rule. Device-facing ports remain caller-owned with their original sizes.
Forwarding shares each output's width among contenders, limits each input to
its input width, and rotates priority in binding order each cycle. Buffering is
one cycle of capacity, not a guarantee of full throughput under contention.
