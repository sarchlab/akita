# switches — Network Switch Component

Package `switches` provides the switch component for the Akita simulation
framework. Within the `noc` networking stack, a switch is the forwarding element
that moves flits from its input ports to its output ports according to a routing
table. Switches are wired together (and to endpoints) by the higher-level
`networkconnector` and topology builders such as mesh, PCIe, and NVLink.

## Key Types

### Comp, Spec, Resources, State

`Comp` is a ticking component, `ticking.Component[Spec, state, Resources, Ports,
middlewares]`. Configuration is split the usual way:

```go
type Spec struct {
    Freq timing.Freq // tick frequency
}

type Resources struct {
    RoutingTable routing.Table // shared, externally owned
    Links        []Link        // the link behind each port
}

type Link struct {
    Remote           messaging.RemotePort // the port at the other end
    Latency          int
    NumInputChannel  int
    NumOutputChannel int
}

type Ports struct {
    Port []messaging.Port // one port per link, aligned with Resources.Links
}
```

`State` holds one `portComplexState` per port — each with an input
pipeline (modeling per-port latency), a route buffer, a forward buffer, and a
send-out buffer (all `queueing` types). The number of input/output channels on a
port controls how many flits may be injected/ejected per cycle.

The middlewares are `RouteForwardSend` and `ReceivePipeline`, run in that order
every cycle.

## How It Works

Each tick the switch runs two middlewares as a five-stage pipeline:

1. **startProcessing** — pull flits from each input port into the port's
   latency pipeline (or directly into the route buffer when latency is 0).
2. **movePipeline** — advance pipelines, draining completed flits into route
   buffers.
3. **route** — look up each flit's final `Dst` in the `routing.Table`, resolve
   it to an output port index, and move it to the forward buffer.
4. **forward** — arbitrate (round-robin across input ports) and copy flits into
   the chosen output port's send-out buffer, one per output port per tick.
5. **sendOut** — send buffered flits out, stamping each flit's hop `Src`/`Dst`
   with the local port and its connected remote port.

## Builder Pattern

A switch takes all of its ports at `Build`: one port per link, created by the
system builder with `twowaybuffered.NewPort` and named `"<instance>.Port[i]"`, and the
matching links in `Resources.Links`. No port is added later, so a switch is
built once its links are known; `networkconnector` builds its switches in
`EstablishRoute`.

```go
sw := switches.Definition.Builder().
    WithSimulation(sim).
    WithSpec(switches.Definition.DefaultSpec).
    WithResources(switches.Resources{
        RoutingTable: rt,
        Links: []switches.Link{
            {Remote: epPort.AsRemote(), Latency: 1, NumInputChannel: 1, NumOutputChannel: 1},
        },
    }).
    Build("Switch0")
sw.BindPort("Port[0]", twowaybuffered.NewPort(1, 1))
```

`WithSimulation` and a non-nil `RoutingTable` are required, and `Links` must have
one entry per port; `Build` panics otherwise.
