# V5 release notes

## Simulation package migration

Runtime packages are now under `sim/`; `simulation`, `monitoring2`, and
`daisen2` are renamed to `sim`, `monitoring`, and `daisen`.

**Live monitoring is now opt-in.** Downstream simulators such as MGPUSim must
supply `WithMonitor(monitoring.NewMonitor())` to retain their monitoring server.
`Build()` binds and starts the supplied monitor, and `Terminate()` stops it.
Omitting `WithMonitor` leaves runs headless while tracing remains independent.
The `WithoutMonitoring` and `WithMonitorPort` options are removed.

Live progress bars move from `daisen.ProgressBar` to `monitoring.ProgressBar`.
The memory access agent accepts a small `ProgressTracker` interface through
`SetProgressTrackers`, keeping UI types out of component code.

See the [migration guide](./tutorial/migration.md) for updated imports,
monitor wiring, and port configuration. MGPUSim migration is separate.

## Switching-network payload delivery

Switching networks preserve the complete original message, including its typed
payload. Only flit 0 carries that message; body flits contain the routing and
reassembly header. Checkpoints retain one copy of the message across flit queues
and reassembly state. `packetization.AssembledMsg` and the `Delivery` role are
removed. Existing checkpoints with the old network layout must be recreated.
Link bandwidth and latency modeling are a separate change.

## Port and connection packages

Buffered ports move to `sim/messaging/twowaybuffered` as `Port`, constructed by
`NewPort(in, out)`. Ideal connections move from `noc/directconnection` to
`sim/messaging/direct` as `Connection`, constructed with
`NewConnection(name, simulation, frequency)`. Frequency is explicit; ports and
connections have no component `Definition`, default spec, or builder. Old paths
and constructors are removed.

NoC builders now explicitly support buffered ports and ideal direct connections.
The port factory is removed; endpoint device wiring rejects unsupported device
ports. Port checkpoint encoding is unchanged.

## Explicit component assembly

Component builders accept Spec and Resources, without `WithPorts`. Build leaves
runtime State and Middlewares unset. Use `component.BindPort("Top", port)` to
assign the slot, owner and full name, and `connection.BindPort(port)` to attach
both sides of a link. Ports use `NewPort(in, out)`; `SetOwner`, `SetConnection`,
and connection `PlugIn` are removed in favor of one-time bindings.

Call `simulation.Initialize()` after wiring and before seeding work, running,
or saving/loading checkpoints. Validation failures leave setup editable; once
initializers begin, topology is frozen. Downstream simulators must migrate this
call order. Domain configuration and builders remain a separate change.
