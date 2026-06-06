# endpoint — Device-to-Network Bridge

Package `endpoint` provides the endpoint component for the Akita simulation
framework. Within the `noc` networking stack, an endpoint bridges a device's
ports and the network: it packetizes outgoing messages into flits and reassembles
incoming flits back into messages. Endpoints sit at the edge of a network built
by the `networkconnector` and the higher-level mesh, PCIe, and NVLink builders.

## Key Types

### Comp, Spec, Resources, State

`Comp` embeds a `modeling.Component[Spec, State, modeling.None]`. The `Spec`
configures conversion and channel behavior:

```go
type Spec struct {
    Freq                  timing.Freq
    NumInputChannels      int                  // flits ejected per tick
    NumOutputChannels     int                  // flits injected per tick
    FlitByteSize          int                  // bytes per flit
    EncodingOverhead      float64              // extra bytes fraction
    DefaultSwitchDst      messaging.RemotePort // first-hop switch port
    NetworkPortBufferSize int
}

type Resources struct {
    DevicePorts []messaging.Port // device ports served by this endpoint
}
```

`State` holds the message-out buffer, the flits queued for sending, and the
per-message reassembly records (`AssemblingMsgs`, `AssembledMsgs`).

`Comp` implements `messaging.Connection`: device ports use the endpoint as their
connection (`PlugIn` calls `SetConnection`), and `NotifySend`/`NotifyAvailable`
wake the component. Key methods:

- `NetworkPort()` / `SetNetworkPort(p)` — the port facing the network.
- `SetDefaultSwitchDst(dst)` — first-hop destination for emitted flits.
- `PlugIn(port)` — attach another device port after build.

## How It Works

Each tick two middlewares run. The outgoing path (`device → network`) retrieves
messages from device ports, converts each into one or more `packetization.Flit`
values — flit count derived from `TrafficBytes`, `EncodingOverhead`, and
`FlitByteSize` — and sends them out the network port with `Dst` set to the
default switch. The incoming path (`network → device`) receives flits, groups
them by message ID, and once `NumFlitInMsg` flits have arrived, reassembles the
`messaging.MsgMeta` and delivers it to the matching device port. Buffers apply
backpressure to keep the serializable state bounded.

## Builder Pattern

```go
ep := endpoint.MakeBuilder().
    WithRegistrar(reg).                                        // *simulation.Simulation or StandaloneRegistrar
    WithSpec(endpoint.DefaultSpec()).
    WithResources(endpoint.Resources{DevicePorts: ports}).
    Build("EndPoint0")
```

`WithRegistrar` is required (`Build` panics otherwise). `Build` creates the
endpoint's network port; device ports listed in `Resources` are plugged in
automatically, and more can be added later with `PlugIn`. `DefaultSpec`
defaults to a 32-byte flit, 0.25 encoding overhead, single input/output
channels, and a network-port buffer of 4.
