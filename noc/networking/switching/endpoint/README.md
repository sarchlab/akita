# endpoint — Device-to-Network Bridge

Package `endpoint` provides the endpoint component for the Akita simulation
framework. Within the `noc` networking stack, an endpoint bridges a device's
ports and the network: it packetizes outgoing messages into flits and reassembles
incoming flits back into messages. Endpoints sit at the edge of a network built
by the `networkconnector` and the higher-level mesh, PCIe, and NVLink builders.

## Key Types

### Comp, Spec, Resources, State

`Comp` is a ticking component, `ticking.Component[Spec, state, Resources, Ports,
middlewares]`. The `Spec` configures conversion and channel behavior:

```go
type Spec struct {
    Freq              timing.Freq
    NumInputChannels  int                  // flits ejected per tick
    NumOutputChannels int                  // flits injected per tick
    FlitByteSize      int                  // bytes per flit
    EncodingOverhead  float64              // extra bytes fraction
    DefaultSwitchDst  messaging.RemotePort // port at the other end of the link
}

type Resources struct {
    DevicePorts []messaging.Port // device ports served by this endpoint
}
```

`State` holds the message-out buffer, the flits queued for sending, and the
per-message reassembly records (`AssemblingMsgs`, `AssembledMsgs`).

`Ports` has one field, `NetworkPort`, the port facing the network. The
middlewares are `Outgoing` (device → network) and `Incoming` (network → device),
run in that order every cycle.

The endpoint is also the connection of its device ports: `Build` plugs every
port in `Resources.DevicePorts` into it, and activity on a device port wakes the
endpoint. No port is added after `Build`.

## How It Works

Each tick the two middlewares run. The outgoing path (`device → network`) retrieves
messages from device ports, converts each into one or more `packetization.Flit`
values — flit count derived from `TrafficBytes`, `EncodingOverhead`, and
`FlitByteSize` — and sends them out the network port with `Dst` set to the
default switch. The incoming path (`network → device`) receives flits, groups
them by message ID, and once `NumFlitInMsg` flits have arrived, reassembles the
`messaging.Msg` and delivers it to the matching device port. Buffers apply
backpressure to keep the serializable state bounded.

## Builder Pattern

```go
spec := endpoint.Definition.DefaultSpec
spec.DefaultSwitchDst = switchPort.AsRemote()

ep := endpoint.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(endpoint.Resources{DevicePorts: ports}).
    WithPorts(endpoint.Ports{
        NetworkPort: messaging.NewPort("EndPoint0.NetworkPort", 4, 4),
    }).
    Build("EndPoint0")
```

`WithSimulation` is required (`Build` panics otherwise). The system builder
creates the network port with `messaging.NewPort`, named
`"<instance>.NetworkPort"`, and sets `Spec.DefaultSwitchDst` to the port at the
other end of the link; `Build` binds and registers the network port and plugs in
the device ports. `DefaultSpec` defaults to a 32-byte flit, 0.25 encoding
overhead, and single input/output channels.
