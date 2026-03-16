# monitoring — Simulation Monitoring Server

Package `monitoring` provides a web-based monitoring dashboard for Akita
simulations. It launches an HTTP server that exposes a web UI and REST API
for inspecting, controlling, and profiling a running simulation.

## Features

- **Web dashboard** — Built-in web UI served from embedded assets.
- **Simulation control** — Pause, continue, and run the simulation engine
  from the browser.
- **Component inspection** — List all registered components and inspect
  their fields/state in JSON.
- **Progress bars** — Create and track named progress bars for long-running
  operations (e.g., kernel dispatch, data transfer).
- **Buffer monitoring** — View all registered buffer fill levels, sorted by
  level or percentage, for hang detection.
- **Resource monitoring** — CPU usage and memory consumption of the
  simulator process.
- **CPU profiling** — Collect and return a 1-second CPU profile on demand.
- **Tracing control** — Start/stop the `tracing.DBTracer` via the API.

## Key Types

### Monitor

```go
type Monitor struct { ... }

func NewMonitor() *Monitor
func (m *Monitor) WithPortNumber(port int) *Monitor
func (m *Monitor) RegisterEngine(e sim.Engine)
func (m *Monitor) RegisterComponent(c sim.Component)
func (m *Monitor) RegisterVisTracer(tr *tracing.DBTracer)
func (m *Monitor) StartServer()
func (m *Monitor) StopServer()
func (m *Monitor) CreateProgressBar(name string, total uint64) *ProgressBar
func (m *Monitor) CompleteProgressBar(pb *ProgressBar)
```

### ProgressBar

```go
type ProgressBar struct {
    ID         uint64
    Name       string
    Total      uint64
    Finished   uint64
    InProgress uint64
}

func (b *ProgressBar) IncrementFinished(amount uint64)
func (b *ProgressBar) IncrementInProgress(amount uint64)
func (b *ProgressBar) MoveInProgressToFinished(amount uint64)
```

## Usage

```go
monitor := monitoring.NewMonitor()
monitor.RegisterEngine(engine)
monitor.RegisterComponent(gpu)
monitor.RegisterComponent(memCtrl)

bar := monitor.CreateProgressBar("Kernels", 100)
monitor.StartServer()

// ... run simulation ...
bar.IncrementFinished(1)

// When done
monitor.CompleteProgressBar(bar)
monitor.StopServer()
```

The server starts on port 32776 by default. If the port is in use, it scans
up to 100 consecutive ports. Use `WithPortNumber(n)` to set a specific port.

## REST API

| Endpoint | Method | Description |
|---|---|---|
| `/api/now` | GET | Current simulation time |
| `/api/run` | GET | Start the simulation engine |
| `/api/pause` | GET | Pause the engine |
| `/api/continue` | GET | Resume the engine |
| `/api/tick/{name}` | GET | Tick a specific component |
| `/api/list_components` | GET | List registered component names |
| `/api/component/{name}` | GET | Inspect a component (JSON) |
| `/api/field/{json}` | GET | Inspect a specific component field |
| `/api/progress` | GET | List active progress bars |
| `/api/resource` | GET | CPU% and memory usage |
| `/api/hangdetector/buffers` | GET | Buffer fill levels (sort, limit, offset) |
| `/api/profile` | GET | 1-second CPU profile |
| `/api/trace/start` | POST | Start DB tracing |
| `/api/trace/end` | POST | Stop DB tracing |
| `/api/trace/is_tracing` | GET | Check if tracing is active |
