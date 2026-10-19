# Akita RTM

Akita RTM (Real-Time Monitor) provides a live web UI for observing running
Akita simulations.

## Attaching to a simulation

Monitoring is opt-in. Construct a monitor and pass it to the simulation builder:

```go
monitor := monitoring.MakeBuilder().WithPortNumber(8080).Build()
s := sim.MakeBuilder().WithMonitor(monitor).Build()
defer func() {
    if err := s.Terminate(); err != nil {
        panic(err)
    }
}()
// Build components and bind their ports here.
if err := s.Initialize(); err != nil { panic(err) }

bar := monitor.CreateProgressBar("work", 100)
// Update bar as work progresses.
monitor.CompleteProgressBar(bar)
```

Import `github.com/sarchlab/akita/v5/monitoring` and
`github.com/sarchlab/akita/v5/sim`. `MakeBuilder().Build()` opens no listener and does not
bind a simulation. Successful `s.Initialize()` calls `Start(s)` after component
State and topology are complete. Start binds runtime services, obtains copied
inventories from `s.Components()` and `s.Ports()`, and begins serving HTTP.
Termination calls `Stop()` before closing recording resources. Failed
initialization does not start the monitor.

Each monitor belongs to one simulation. A second `Start` panics before rebinding
or opening another listener, even after `Stop()`. Create progress bars only after
`Initialize()` has started the monitor; earlier calls panic with a clear diagnostic.

Omit `WithMonitor` for headless runs. Visualization tracing is configured
independently. Keep the concrete `monitor` variable for progress bars and other
implementation-specific features. The exported `sim.Monitor` contract contains
only `Start(*sim.Simulation)` and `Stop()`.

Buffer inspection reads ports through `IncomingBuf()`/`OutgoingBuf()` and omits
absent buffers. It also discovers direct component buffers and buffers in State,
including buffer slices. Requests inspect at an engine boundary and resolve State
afresh so restored buffers remain visible. Capacities are the actual configured
capacities, independent of occupancy.

`ProgressBar` belongs to `monitoring`. Neither the monitor nor its progress bars
depend on the offline Daisen viewer.

Read progress through `bar.Snapshot()`, which returns a consistent independent
`ProgressBarSnapshot`. Use `IncrementInProgress`, `IncrementFinished`, and
`MoveInProgressToFinished` to update counters. The monitor initializes the start
time when creating a bar; its HTTP JSON fields are unchanged. Completing an
already-removed bar is safe. Importing monitoring does not register handlers on
`http.DefaultServeMux`; profiling remains on the monitor's own endpoints.
