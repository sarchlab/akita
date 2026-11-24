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
pipeline (modeling per-port latency), a shared ingress waiting buffer, and a
send-out buffer (all `queueing` types). The number of input/output channels on a
port controls how many flits may be injected/ejected per cycle.

The middlewares are `RouteForwardSend` and `ReceivePipeline`, run in that order
every cycle.

## How It Works

Each tick the switch runs two middlewares as a five-stage pipeline:

1. **startProcessing** resolves each admitted flit's destination and puts it in
   the input latency pipeline. A zero-latency input goes directly into waiting.
2. **movePipeline** drains ready flits into the shared ingress buffer. When only
   the pipeline is active, the switch sleeps until a flit becomes ready.
3. **route** completes one routing cycle in place. The eligible prefix of the
   waiting buffer can arbitrate starting on the next tick.
4. **forward** visits ingress ports in round-robin order. Within an ingress, it
   forwards the oldest eligible flits whose outputs have credit, preserving FIFO
   order for each destination output. Input and output channel counts bound the
   number forwarded on each side per tick. All outputs share twice the input
   width in waiting slots per ingress. A blocked output can occupy every slot.
5. **sendOut** sends buffered flits and stamps each hop's source and destination.

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
