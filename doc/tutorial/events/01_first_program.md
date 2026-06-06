---
sidebar_position: 1
---

# Scheduling a Single Event

Components are the default way to build an Akita simulation, but they sit
on top of a smaller and more general primitive: **events** dispatched to
**handlers** by an engine that owns the simulated clock. This chapter
opens that lower layer.

You will not normally write code at this level for a real model — write a
component instead. Reach for raw events when you need ad-hoc behaviour
that does not justify a component, when you are writing test scaffolding,
or when you simply want to understand what the engine is doing underneath.

The simplest such program creates an engine, schedules one event, and
watches the handler print a single line.

The full source is in `examples/01_print_event/main.go`.

## What You Will Learn

- How to create a simulation and get its engine.
- What an event is and how to schedule one.
- What a handler is and how to register it.
- How the engine runs until the event queue is empty.

## Walk-Through

### 1. The handler

A handler is anything that satisfies `timing.Handler`, which means it
implements a single method:

```go
type EventPrinter struct{}

func (e *EventPrinter) Handle(event timing.Event) error {
    fmt.Printf("Event: %d\n", event.Time())
    return nil
}
```

When the engine fires an event, it looks up the registered handler by name
and calls `Handle(event)`. Anything the handler returns as an error stops
the simulation.

### 2. Building the simulation

```go
s := simulation.MakeBuilder().Build()
```

`simulation.MakeBuilder().Build()` returns a `*simulation.Simulation`. It
owns an engine, a registrar, and optional tracing and monitoring
infrastructure. For this example we only need the engine:

```go
engine := s.GetEngine()
```

### 3. Registering the handler

The engine routes events to handlers by **name**, not by pointer. Register
the handler under a name you choose:

```go
if registrar, ok := engine.(timing.HandlerRegistrar); ok {
    registrar.RegisterHandler("printer", handler)
}
```

The type assertion is a safety check — most engines implement
`HandlerRegistrar`, but the interface keeps that explicit.

### 4. Creating and scheduling the event

```go
evt := timing.MakeEventBase(1, "printer")
engine.Schedule(evt)
```

`MakeEventBase(time, handlerID)` creates a minimal event whose `Time()`
returns `1` and whose `HandlerID()` returns `"printer"`. The engine will
fire it at time = 1 picosecond and dispatch it to the handler registered
under `"printer"`.

### 5. Running

```go
err := engine.Run()
if err != nil {
    panic(err)
}
```

`Run()` pulls events off the queue in time order, dispatches each to its
handler, and returns when the queue is empty. Here it fires exactly one
event and returns.

### 6. Cleanup

```go
s.Terminate()
```

`Terminate` flushes any tracing and data-recording buffers held by the
simulation. Always call it before the program exits, even when there is
nothing to flush.

## Run It

```bash
cd examples/01_print_event
go run main.go
```

Output:

```
Event: 1
```

The engine fired the event at time = 1 picosecond, the handler printed,
and the engine returned because no further events were scheduled.

## Key Concepts

- **Time is unsigned 64-bit picoseconds** (`timing.VTimeInPicoSec`). Despite
  the name, the unit is picoseconds. 1 second = 1,000,000,000,000.
- **Handlers are registered by name**, not by reference. This is what
  lets the engine work with serializable state.
- **An empty event queue ends the run.** There is no fixed end time; you
  control when it ends by deciding what events to schedule.

## Where to Next

The next chapter shows handlers that schedule *more* events during the
simulation — the basic pattern that components themselves use internally.
