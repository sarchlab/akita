# V5 release notes

## Simulation package migration

Runtime packages are now under `sim/`; `simulation`, `monitoring2`, and
`daisen2` are renamed to `sim`, `monitoring`, and `daisen`.

**Live monitoring is now opt-in.** Downstream simulators such as MGPUSim must
supply `WithMonitorFactory` to retain their monitoring server. `Build()` starts
the supplied monitor, and `Terminate()` stops it. `WithoutMonitoring()` skips
monitor construction and startup while leaving tracing independent.

See the [migration guide](./tutorial/migration.md) for updated imports,
monitor construction, and port configuration. MGPUSim migration is separate.
