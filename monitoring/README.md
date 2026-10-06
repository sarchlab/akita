# Akita RTM

Akita RTM (Real-Time Monitor) provides a live web UI for observing running
Akita simulations.

## Attaching to a simulation

Monitoring is opt-in. Construct a monitor and pass it to the simulation builder:

```go
monitor := monitoring.NewMonitor().WithPortNumber(8080)
s := sim.MakeBuilder().WithMonitor(monitor).Build()
defer s.Terminate()

bar := monitor.CreateProgressBar("work", 100)
// Update bar as work progresses.
monitor.CompleteProgressBar(bar)
```

Import `github.com/sarchlab/akita/v5/monitoring` and
`github.com/sarchlab/akita/v5/sim`. `NewMonitor()` opens no listener and does not
bind a simulation. `Build()` calls `Start(s)` after initializing the engine,
tracer, and recorder. Start permanently binds those services and begins serving
HTTP. The runner forwards component and port registrations and calls `Stop()`
before closing recording resources during termination.

Each monitor belongs to one simulation. A second `Start` panics before rebinding
or opening another listener, even after `Stop()`. Create progress bars only after
`Build()` has started the monitor; earlier calls panic with a clear diagnostic.

Omit `WithMonitor` for headless runs. Visualization tracing is configured
independently. `s.Monitor()` exposes the small `sim.Monitor` interface, or nil
when monitoring is disabled; keep the concrete `monitor` variable for progress
bars and other implementation-specific features.

`ProgressBar` belongs to `monitoring`. Neither the monitor nor its progress bars
depend on the offline Daisen viewer.
