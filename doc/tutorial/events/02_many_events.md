---
sidebar_position: 2
---

# Many Events

A single event is a one-shot. Real simulations have handlers that schedule
*more* events while running — that is how anything interesting unfolds over
time. This chapter models a population of cells, each of which periodically
splits into two.

The full source is in `examples/02_cell_split/main.go`.

## What You Will Learn

- How to define a **custom event type** with your own payload.
- How a handler can schedule new events.
- How to terminate a run when a logical condition is met.

## The Model

There is one cell at time 0. Each existing cell schedules a "split" event
at a random future time between 1 and 2 seconds. When the split event
fires, that cell becomes two cells, and each schedules its own next split.
The number of live cells grows exponentially. We stop after 10 simulated
seconds.

## Walk-Through

### 1. A custom event type

Anything that implements `timing.Event` can be scheduled. The interface
has three methods:

```go
type splitEvent struct {
    time      timing.VTimeInPicoSec
    handlerID string
    id        int
}

func (e splitEvent) Time() timing.VTimeInPicoSec { return e.time }
func (e splitEvent) HandlerID() string       { return e.handlerID }
func (e splitEvent) IsSecondary() bool       { return false }
```

The `id` field is **your payload** — handlers can read whatever fields
you put on the event. `IsSecondary()` returns `false` for normal events;
secondary events are an advanced feature for end-of-tick processing.

### 2. The handler

```go
type handler struct {
    count int
}

func (h *handler) Handle(e timing.Event) error {
    h.count += 1

    evt := e.(splitEvent)
    fmt.Printf("Cell %d split at %d ps, current count: %d\n",
        evt.id, evt.Time(), h.count)

    h.scheduleNextSplitEvent(evt.Time(), evt.id)
    h.scheduleNextSplitEvent(evt.Time(), h.count)

    return nil
}
```

Two important things here:

- The handler **mutates its own state** (`h.count++`). This is fine —
  handlers are ordinary Go code.
- After processing, it **schedules two new events** (one for the original
  cell, one for the newly-born one). That is what makes the simulation
  continue.

### 3. Scheduling the next event

```go
func (h *handler) scheduleNextSplitEvent(now timing.VTimeInPicoSec, id int) {
    timeUntilNextSplit := timing.VTimeInPicoSec(uint64((randGen.Float64() + 1) * 1e12))
    nextEvt := splitEvent{
        time:      now + timeUntilNextSplit,
        handlerID: "splitter",
        id:        id,
    }

    if nextEvt.time < endTime {
        engine.Schedule(nextEvt)
    }
}
```

`(rand + 1) * 1e12` produces a delay between 1 and 2 picoseconds of
floating-point — but multiplied here it gives 1e12 to 2e12 picoseconds
(1 to 2 simulated seconds). The `if nextEvt.time < endTime` check is the
stop condition: events scheduled past 10 seconds are simply dropped, so
when no more events are pending, the engine returns.

### 4. Main

```go
randGen = rand.New(rand.NewSource(0))

s := simulation.MakeBuilder().Build()
engine = s.GetEngine()
h := handler{count: 1}

if registrar, ok := engine.(timing.HandlerRegistrar); ok {
    registrar.RegisterHandler("splitter", &h)
}

firstEvtTime := timing.VTimeInPicoSec(uint64((randGen.Float64() + 1) * 1e12))
firstEvt := splitEvent{
    time:      firstEvtTime,
    handlerID: "splitter",
    id:        0,
}
engine.Schedule(firstEvt)

err := engine.Run()
```

Same shape as the previous example: build, get engine, register handler,
schedule the first event, run. Using a seeded `rand.New(rand.NewSource(0))`
makes the run deterministic — the same seed produces the same trace.

## Run It

```bash
cd examples/02_cell_split
go run main.go
```

Tail of output:

```
...
Cell 3 split at 9990583990560 ps, current count: 75
Cell count at time 10000000000000 ps: 75
```

(Your exact numbers will match, given the fixed seed.) Notice how the
population grows: every split creates cells that themselves split, so the
count climbs exponentially.

## Key Concepts

- **Events carry data.** Anything implementing `timing.Event` works — add
  whatever fields you need.
- **Handlers drive the simulation forward** by scheduling new events.
  Without that, a simulation would run a fixed list of events and stop.
- **Termination is implicit.** The simulation ends when the event queue
  becomes empty. You control that by deciding what gets scheduled.
- **Determinism comes from seeded randomness** and a serial engine. Same
  seed, same code, same output, run after run.

## Where to Next

So far events are bare functions over global state. The next chapter
introduces **event-driven components**, which wear the component shape you
already know from the first section — Spec, State, ports — but wake on
demand instead of every cycle. They are the bridge between raw events and
the default ticking components.
