# switches — Network Switch Component

Package `switches` provides the switch component for the Akita simulation
framework. Within the `noc` networking stack, a switch is the forwarding element
that moves flits from its input ports to its output ports according to a routing
table. Switches are wired together (and to endpoints) by the higher-level
`networkconnector` and topology builders such as mesh, PCIe, and NVLink.

## Key Types

### Comp, Spec, Resources, State

`Comp` is a `modeling.Component[Spec, State, modeling.None]`. Configuration is
split the usual way:

```go
type Spec struct {
    Freq timing.Freq // tick frequency
}

type Resources struct {
    RoutingTable routing.Table // shared, externally owned
}
```

`State` holds one `portComplexState` per added port — each with an input
pipeline (modeling per-port latency), a route buffer, a forward buffer, and a
send-out buffer (all `queueing` types). The number of input/output channels on a
port controls how many flits may be injected/ejected per cycle.

`GetRoutingTable(comp)` returns the routing table the switch uses, located by
middleware type rather than index.

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

The switch is built first, then ports are added one at a time.

```go
sw := switches.MakeBuilder().
    WithRegistrar(reg).                                  // *simulation.Simulation or StandaloneRegistrar
    WithSpec(switches.DefaultSpec()).
    WithResources(switches.Resources{RoutingTable: rt}).
    Build("Switch0")

switches.MakeSwitchPortAdder(sw).
    WithPorts(localPort, remotePort).
    WithLatency(1).
    WithNumInputChannel(1).
    WithNumOutputChannel(1).
    AddPort()
```

`WithRegistrar` and a non-nil `RoutingTable` are required — `Build` panics
otherwise. `MakeSwitchPortAdder` defaults to one input/output channel and a
latency of 1; `WithPorts` takes the switch-side local port and the remote port
it connects to (an endpoint's or another switch's port).
