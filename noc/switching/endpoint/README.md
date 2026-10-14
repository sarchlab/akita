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

The endpoint becomes the connection of its device ports when
`endpoint.ConnectDevices(ep)` attaches every port in `Resources.DevicePorts`
during wiring. Bind the device owners first, call `ConnectDevices`, and finish
the network links before `simulation.Initialize()`. Activity on a connected
device port then wakes the endpoint.

## How It Works

Each tick the two middlewares run. The outgoing path (`device → network`) retrieves
messages from device ports, converts each into one or more `packetization.Flit`
values — flit count derived from `TrafficBytes`, `EncodingOverhead`, and
`FlitByteSize` — and sends them out the network port with `Dst` set to the
default switch. The incoming path (`network → device`) receives flits, groups
them by message ID, and records each sequence number. Only flit 0 carries the
original `messaging.Msg`; it can arrive before or after the other flits. Once
all flits have arrived, the endpoint delivers that original message unchanged
to the matching device port. The received-sequence tracking and retained message
are checkpointed, including while delivery is blocked. Buffers apply
backpressure to keep the serializable state bounded.

## Builder Pattern

```go
spec := endpoint.Definition.DefaultSpec
spec.DefaultSwitchDst = switchPort.AsRemote()

ep := endpoint.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(endpoint.Resources{DevicePorts: ports}).
    Build("EndPoint0")
ep.BindPort("NetworkPort", twowaybuffered.NewPort(4, 4))
endpoint.ConnectDevices(ep) // owners of DevicePorts must already be bound
```

`WithSimulation` is required (`Build` panics otherwise). The system builder
creates an unnamed network port with `twowaybuffered.NewPort` and sets `Spec.DefaultSwitchDst` to the port at the
other end of the link. `ep.BindPort` binds the network port;
`endpoint.ConnectDevices` attaches the device ports before `simulation.Initialize`. `DefaultSpec` defaults to a 32-byte flit, 0.25 encoding
overhead, and single input/output channels.

Device ports must be `*twowaybuffered.Port`. ConnectDevices rejects unsupported and nil
device ports before attaching any device port. The network connector constructs
buffered network-facing ports and connects them with ideal `direct.Connection`
links; wire ports are not supported.

This restriction reflects the v5 NoC transport support policy. The endpoint
requires buffered push delivery: it drains outgoing messages using
`PeekOutgoing`/`RetrieveOutgoing`, checks incoming capacity with `CanDeliver`,
and pushes each reassembled message into the destination's incoming buffer with
`Deliver`. Device ports must notify the endpoint when outgoing data arrives or
incoming capacity becomes available, so it can resume after backpressure.

The planned wire ports in [#496](https://github.com/sarchlab/akita/issues/496)
use pull delivery: a receiver reads its peer's outgoing slot through the wire,
without an incoming buffer. Their push-side operations are unsupported, even
though they implement the same `messaging.Port` interface. Method signatures
alone therefore do not guarantee compatibility with this endpoint.

A custom port could implement the required buffered push semantics, but v5 NoC
support is intentionally limited to `*twowaybuffered.Port`. Component port
fields remain `messaging.Port` so components can be used with other transports
outside this NoC configuration.
