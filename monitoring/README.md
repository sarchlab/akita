# Akita RTM

Akita RTM (Real-Time Monitor) provides a live web UI for observing running
Akita simulations.

## Attaching to a simulation

Monitoring is opt-in. Supply a factory so each simulation gets its own monitor:

```go
s := sim.MakeBuilder().
    WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
        return monitoring.NewMonitor(s).WithPortNumber(8080)
    }).Build()
defer s.Terminate()
```

Import `github.com/sarchlab/akita/v5/monitoring` and
`github.com/sarchlab/akita/v5/sim`. `NewMonitor(s)` permanently binds the monitor
to the simulation and its recording resources without opening a listener.
The runner calls `Start()`, forwards component and port registrations, and calls
`Stop()` before closing recording resources during termination. A second
`Start()` panics, even after `Stop()`; monitors cannot be reused.

`WithoutMonitoring()` skips factory invocation and server startup, regardless
of builder option order. It does not disable visualization tracing.

For implementation-specific APIs such as progress bars, keep the concrete
monitor in application setup:

```go
var monitor *monitoring.Monitor
s := sim.MakeBuilder().WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
    monitor = monitoring.NewMonitor(s)
    return monitor
}).Build()
defer s.Terminate()
bar := monitor.CreateProgressBar("work", 100)
// Update bar as work progresses.
monitor.CompleteProgressBar(bar)
```

Do not access the captured monitor when monitoring is disabled, since the
factory is not called. `s.Monitor()` returns nil in that case.
