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

Spec holds the switch frequency. Resources references network wiring, which
provides per-port processing parameters and routes derived at Initialize.
The component's `Ports.Links` is the sole runtime port list; its binding order
determines arbitration order. Changing routing
tables after initialization is not supported.

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

## Assembly and checkpoints

Applications use the public NoC connectors. Internally, AddSwitch builds the
component with its network resource, and connect calls bind ports immediately.
State initialization allocates port complexes after checking completed wiring.
Middleware captures the derived routing table and ports in binding order.

Per-port latency still models the pipeline inside the switch, and channel counts
still limit admission/ejection. They have not been reinterpreted as physical link
latency or bandwidth. Non-ideal transport is a separate follow-up.

The network's registered topology resource validates routes, wiring, configuration
and arbitration order on checkpoint restore. Mutable queues and the round-robin
cursor remain in State. Runtime route changes are not supported by this refactor;
a future adaptive-routing model must checkpoint those changes.
