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
        NumInputChannel: 1, NumOutputChannel: 1,
    },
    SwitchEndParam: networkconnector.LinkEndSwitchParameter{
        NumInputChannel: 1, NumOutputChannel: 1, Latency: 2,
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

`NumInputChannel` and `NumOutputChannel` are positive flits-per-cycle limits.
`Latency` is nonnegative processing latency inside the receiving switch.
Network-owned incoming ports, route queues, and forward queues each hold
`NumInputChannel` flits. Send queues and outgoing ports hold `NumOutputChannel`
flits. Endpoint network ports follow the same input/output rule. Device ports
supplied by the caller retain their own capacities. The topology wrappers
provide width defaults; hand-written end parameters must specify both widths.

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
