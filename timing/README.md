# timing — Discrete-Event Simulation Core

Package `timing` provides simulation time, events, and event engines for the
Akita simulation framework. It is the discrete-event kernel that every other
package builds on: an engine holds a priority queue of events, advances virtual
time, and dispatches each event to the handler that owns it.

## Key Concepts

- **Virtual time** is represented by `VTimeInPicoSec`, a `uint64` counting
  picoseconds of simulated time. The engine always moves time forward.
- **Events** describe something that will happen at a future time. Each event
  is bound to exactly one handler.
- **Handlers** define a domain for events. An event may only be scheduled by,
  and may only directly modify, its handler.
- The **engine** repeatedly pops the earliest event, sets the current time to
  that event's time, and calls the handler to process it, until no events
  remain.

## Key Types

### Event and EventBase

```go
type Event interface {
    Time() VTimeInPicoSec   // when the event happens
    HandlerID() string  // name of the handler that processes it
    IsSecondary() bool  // secondary events run after same-time primary events
}
```

Embed `EventBase` to get the standard fields and getters. `MakeEventBase(id, t, handlerID)`
returns an `EventBase` value with the supplied ID:

```go
type tickEvent struct {
    timing.EventBase
}

evt := tickEvent{timing.MakeEventBase(sim.NewID(), now, comp.Name())}
```

### Handler

```go
type Handler interface {
    Handle(e Event)
}
```

### Engine, EventScheduler, TimeTeller

```go
type TimeTeller interface {
    CurrentTime() VTimeInPicoSec
}

type EventScheduler interface {
    TimeTeller
    Schedule(e Event)
}

type Engine interface {
    hooking.Hookable
    EventScheduler
    Run() error
    RequestPause() PauseRequest
    Pause() error
    Continue() error
    IsPaused() bool
    State() EngineState
    Inspect(context.Context, func() error) error
}
```

`SerialEngine` runs events strictly one after another and is deterministic.
`ParallelEngine` runs same-time, non-conflicting events across goroutines.
See [pause and boundary inspection](CONTROL.md) for acknowledged controls and
safe snapshots.

Both implement `Engine`, register handlers by name via `RegisterHandler`, and
keep a separate secondary queue for `IsSecondary()` events.

```go
engine := timing.NewSerialEngine()
engine.RegisterHandler(comp.Name(), comp)
engine.Schedule(evt)
err := engine.Run()
```

## Frequency

`Freq` expresses a clock rate in Hz, with the units `Hz`, `KHz`, `MHz`, and
`GHz`. It converts between time and cycles and snaps times onto tick
boundaries.

```go
freq := 1 * timing.GHz
period := freq.Period()           // picoseconds between ticks
next := freq.NextTick(now)        // next tick strictly after now
this := freq.ThisTick(now)        // now rounded up to a tick boundary
later := freq.NCyclesLater(3, now)
```

## ID Generation

Each simulation owns an atomic ID counter. Use `sim.NewID()` to allocate a
`uint64` ID. Components retain their simulation, so middleware can call
`comp.Simulation().NewID()`. Engines schedule events and do not allocate IDs.
The first ID is 1; zero remains unset. Separate simulations can reuse the same
numeric IDs. Parallel callers receive unique IDs within their simulation, with
allocation order determined by execution order.

Event factories accept the allocated value:

```go
evt := timing.MakeEventBase(sim.NewID(), when, comp.Name())
```

Events and messages store IDs without retaining the simulation. The
process-global generator, configuration, and reset functions have been removed.

A `simulation.Simulation` checkpoints its counter automatically. Lightweight
setups can create `sim := modeling.NewStandaloneSimulation(engine)` once and
share it with every component builder. They must save and restore
`sim.GetIDGenerator()` alongside the engine and other entities. Restore into a
fresh, stopped simulation; restoring one simulation does not change another
simulation's counter. Parallel simulation checkpointing remains unsupported.

## Hooks

Engines are `hooking.Hookable`. They fire `HookPosBeforeEvent` and
`HookPosAfterEvent` around each event, with the event passed as `HookCtx.Item`.
Attach a hook with `engine.AcceptHook(myHook)` to observe or trace execution.
