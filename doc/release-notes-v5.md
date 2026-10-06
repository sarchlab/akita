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
`NewPort(name, in, out)`. Ideal connections move from `noc/directconnection` to
`sim/messaging/direct` as `Connection`, built with `Definition.Builder()` and
configured from `Definition.DefaultSpec`. Old paths and constructors are removed.

NoC builders now explicitly support buffered ports and ideal direct connections.
The port factory is removed; endpoint construction rejects unsupported device
ports. Naming, owner binding, and port checkpoint encoding are unchanged.
