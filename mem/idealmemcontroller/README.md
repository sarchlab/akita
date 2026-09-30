# idealmemcontroller — Ideal Memory Controller

Package `idealmemcontroller` provides a simplified memory controller for the
Akita simulation framework. Every request completes after a fixed number of
cycles with no bandwidth or concurrency limitations, making it a good fit for
testing and for simulations where memory-timing detail is not important.

## How It Works

The controller processes `mem.ReadReq` and `mem.WriteReq` messages on its `Top`
port. Each request is assigned a countdown timer equal to the configured
`Latency` (in cycles). Every tick, all inflight countdowns decrement by one.
When a countdown reaches zero, the controller reads from or writes to its
backing `mem.Storage` and sends the response.

There is no limit on the number of concurrent inflight transactions — all
requests proceed in parallel. At most `Width` new requests are accepted per
tick.

Two middlewares, the fields of `Middlewares`, run each tick in this order:

| Middleware | Role |
|---|---|
| **Ctrl** | Handles `memcontrolprotocol.Req` on the `Control` port (enable, pause, drain, reset). |
| **Memory** | Accepts new read/write requests (up to `Width` per tick), decrements countdowns, performs storage I/O, and sends responses. |

### Controller States

| State | Behavior |
|---|---|
| `"enable"` | Normal operation — accepts new requests and processes countdowns. |
| `"pause"` | Stops accepting new requests and freezes all countdowns. |
| `"drain"` | Stops accepting new requests but continues processing inflight transactions. Sends `mem.ControlRsp` when all are complete, then transitions to `"pause"`. |

## Key Types

- `Spec` — immutable configuration: frequency, latency, width, and cache-line
  size.
- `State` — mutable runtime data: the list of inflight transactions and the
  current control state (`"enable"`, `"pause"`, or `"drain"`).
- `Resources` — shared wiring, supplied by the system builder; holds the
  backing `*mem.Storage`, which is required.
- `Ports` — the `Top` and `Control` ports.
- `Middlewares` — `Ctrl` and `Memory`, run in that order every cycle.
- `Comp` — `ticking.Component[Spec, State, Resources, Ports, Middlewares]`, a
  ticking component.

```go
type Spec struct {
    Freq          timing.Freq // Operating frequency
    Width         int         // Max new requests accepted per tick
    Latency       int         // Fixed response latency in cycles
    CacheLineSize int         // Access granularity in bytes
}
```

The controller does not size or create its storage: the system builder
creates the `mem.Storage` with the capacity it needs (for example with
`mem.MakeStorageBuilder().WithCapacity(4 * mem.GB).WithSimulation(sim)`, which
also registers it with the simulation so it is checkpointed) and passes it in
`Resources`.

The controller uses **global storage**: a request's address indexes the backing
store directly, with no per-controller address conversion. When several
controllers are interleaved over one shared `mem.Storage`, the sender's
destination map selects the controller; each controller simply reads and writes
the shared store at the request's global address.

## Builder Pattern

Start from `Definition.DefaultSpec`, tweak the fields you need, and pass the
whole spec to `WithSpec`. Wiring comes from `WithSimulation` (which provides the
engine and registers the component), `WithResources` (the backing storage,
required), and `WithPorts` (the port instances).

```go
spec := idealmemcontroller.Definition.DefaultSpec
spec.Latency = 50

storage := mem.MakeStorageBuilder().
    WithCapacity(4 * mem.GB).
    WithSimulation(sim).
    Build("IdealMem.Storage")

ctrl := idealmemcontroller.Definition.Builder().
    WithSimulation(sim).
    WithSpec(spec).
    WithResources(idealmemcontroller.Resources{Storage: storage}).
    WithPorts(idealmemcontroller.Ports{
        Top:     messaging.NewPort(nil, 16, 16, "IdealMem.Top"),
        Control: messaging.NewPort(nil, 16, 16, "IdealMem.Control"),
    }).
    Build("IdealMem")

topPort := ctrl.Ports.Top
```

| Method | Description |
|---|---|
| `WithSimulation(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `Definition.DefaultSpec` and tweak |
| `WithResources(Resources{Storage: s})` | Backing storage, possibly shared (required) |
| `WithPorts(Ports{...})` | The port instances, each named `"<instance>.<field>"` (required) |

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Latency | 100 cycles |
| Width | 1 request/tick |
| Cache line size | 64 bytes |

## Ports

The system builder creates each port with `messaging.NewPort`, choosing its
buffer sizes, and passes them to `WithPorts`; `Build` binds and registers them.

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns `mem.DataReadyRsp`
  and `mem.WriteDoneRsp`.
- **Control**: accepts `memcontrolprotocol.Req` (enable / pause / drain /
  reset), returns `memcontrolprotocol.Rsp`.
