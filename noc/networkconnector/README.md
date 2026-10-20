# Generic ideal-network connector

`networkconnector` assembles endpoints, switches, and ideal direct connections.
Device ports must be `*twowaybuffered.Port`. Physical transport bandwidth and
latency are not modeled; switch processing widths, latency, and buffering are.

```go
network := networkconnector.MakeBuilder().WithSimulation(s).
    WithFreq(timing.GHz).WithFlitSize(64).Build("Network")
sw := network.AddSwitch(networkconnector.SwitchConfig{Name: "Switch"})
network.ConnectDevice(sw, devicePorts, networkconnector.DeviceConfig{
    Name: "GPU0",
    DeviceEndParam: networkconnector.LinkEndDeviceParameter{
        IncomingBufSize: 8, OutgoingBufSize: 8,
        NumInputChannel: 1, NumOutputChannel: 1,
    },
    SwitchEndParam: networkconnector.LinkEndSwitchParameter{
        IncomingBufSize: 8, OutgoingBufSize: 8,
        NumInputChannel: 1, NumOutputChannel: 1, Latency: 2,
        RouteBufferCapacity: 4, ForwardBufferCapacity: 4, SendBufferCapacity: 4,
    },
})
if err := s.Initialize(); err != nil { panic(err) }
```

`AddSwitch(SwitchConfig)` accepts an optional name and `*routing.Table`.
An empty name is generated; a nil table creates an independent table.
`ConnectDevice(switchID, ports, DeviceConfig)` accepts an optional endpoint name.
Repeated explicit names append ports only when switch and configuration match.
`ConnectSwitches(a, b, SwitchToSwitchLinkParameter)` takes a separate
`LinkEndSwitchParameter` for each end. No operation returns internal ports.

`NumInputChannel` and `NumOutputChannel` are flits per cycle, not buffer sizes.
`Latency` is processing inside the receiving switch, not physical propagation.
All port and switch queue capacities are independent positive counts of flits.
Switch latency must be nonnegative. The topology wrappers expose corresponding
direct setters and provide defaults; hand-written end parameters specify all
required widths and queue capacities explicitly.

## Routing and lifecycle

`WithRouter` retains the public setup-router extension. `FloydWarshallRouter`
selects fewest-hop routes by default. Use `WithRouter(nil)` with supplied routing
tables to keep manually defined routes. A non-nil router runs at initialization
and may populate the supplied tables. `Node`, `Remote`, and the concrete
`routing.Table` remain public. There is no bandwidth-first router or
`Remote.Bandwidth` method for the ideal-only model.

All connect calls precede `s.Initialize()`. It derives routes from bound ports,
rejects disconnected networks, unreachable destinations, and routing loops, and
copies routes for runtime use. Changing source tables after initialization does
not change runtime routing. Endpoint/switch implementation packages are internal.

A network-owned checkpoint record fingerprints wiring, binding order, queue
settings, and computed routes. Runtime switch queues are checkpointed separately.
Reusing a builder creates independent networks. No explicit finalization or reset
method exists. Wire ports and non-ideal links remain unsupported.
