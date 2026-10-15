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
    Freq         timing.Freq         `json:"freq"`
    Links        []Link              `json:"links"`
    Routes       []routing.Route     `json:"routes"`
    DefaultRoute messaging.RemotePort `json:"default_route"`
}

type Resources struct{}

type Link struct {
    Remote           messaging.RemotePort `json:"remote"`
    Latency          int                  `json:"latency"`
    NumInputChannel  int                  `json:"num_input_channel"`
    NumOutputChannel int                  `json:"num_output_channel"`
}

type Ports struct {
    Links []messaging.Port // same index as Spec.Links
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

A switch receives its final links and routing configuration in Spec at Build.
The system builder then creates one unnamed port per link and binds each indexed
slot. Finish all bindings before `simulation.Initialize()`.

```go
spec := switches.DefaultSpec()
spec.Links = []switches.Link{
    {Remote: "Endpoint.Network", Latency: 1, NumInputChannel: 1, NumOutputChannel: 1},
}
spec.Routes = []routing.Route{{Destination: "Device.Top", Output: "Switch0.Links[0]"}}
sw := modeling.MakeBuilder[*switches.Comp](spec).WithSimulation(sim).Build("Switch0")
sw.Ports.Links = make([]messaging.Port, len(spec.Links))
sw.BindPort("Links[0]", twowaybuffered.NewPort(1, 1))
```

Build validates flat record shape. The State initializer checks link parameters
and `len(Spec.Links) == len(Ports.Links)` before allocating port complexes.
A mismatch panics and leaves the simulation unusable; correct the configuration
and rebuild. Generic missing-port errors detected before element initialization
still leave setup editable.
Routing middleware derives its private table from `Spec.Routes` and
`Spec.DefaultRoute`. There is no Resources routing override. The connector
computes routes from planned port names before building switches, then binds
ports and completes pending links.
