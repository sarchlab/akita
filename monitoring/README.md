# Akita RTM

Akita RTM (Real-Time Monitor) provides a live web UI for observing running
Akita simulations.

This README is a placeholder. Documentation for Akita RTM will be added here.

## Attaching to a simulation

Monitoring is opt-in. Configure the server here and inject it into the runner:

```go
monitor := monitoring.NewMonitor().WithPortNumber(8080)
sim := simulation.MakeBuilder().WithMonitor(monitor).Build()
defer sim.Terminate()
```

Import `github.com/sarchlab/akita/v5/monitoring` and
`github.com/sarchlab/akita/v5/simulation`. The runner starts the monitor,
forwards component and port registrations, and stops it during termination.
Keep `monitor` when application code needs progress bars or other web-specific
features. A monitor instance must not be shared between simulations.
