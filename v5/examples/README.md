# examples — Akita Example Programs

This directory contains example programs demonstrating how to use the Akita
simulation framework. They progress from minimal event scheduling to full
component communication patterns.

## Examples

### 01_print_event — Basic Event Scheduling

The simplest Akita program. Creates an engine, registers an event handler,
schedules one event, and runs the simulation.

**Key concepts**: `sim.Engine`, `sim.Event`, `sim.Handler`, event scheduling.

```bash
cd 01_print_event && go run main.go
```

### 02_cell_split — Exponential Event Growth

Simulates cell splitting: each cell schedules two split events at random
future times, creating exponential growth. Demonstrates dynamic event
scheduling where handlers create new events during execution.

**Key concepts**: Custom event types, recursive event scheduling, random
timing.

```bash
cd 02_cell_split && go run main.go
```

### tickingping — Tick-Based Component Communication

Two components exchange ping/pong messages using the **ticking middleware**
paradigm (`modeling.Component[Spec, State]`). Each component has two
middlewares:

- **sendMW** — Sends a `PingMsg` each tick until `NumPingPerCycle` is
  reached.
- **receiveProcessMW** — Receives incoming `PingMsg` messages from the port
  and sends `PongMsg` responses.

**Key concepts**: `modeling.Component`, `Spec`/`State` separation, middleware
pattern, port-based communication, builder pattern.

```bash
cd tickingping && go test -v -run Example
```

### ping — Event-Driven Component Communication

Two components exchange ping/pong messages using the **event-driven**
paradigm (`modeling.EventDrivenComponent[Spec, State]`). A single
`PingProcessor` handles all logic: sending scheduled pings, processing
incoming requests, and delivering responses after a configurable delay.

**Key concepts**: `modeling.EventDrivenComponent`, `EventProcessor` interface,
delayed response delivery, event-driven (non-ticking) design.

```bash
cd ping && go test -v -run Example
```

## Choosing a Paradigm

| Paradigm | Component Type | When to Use |
|---|---|---|
| **Ticking** | `modeling.Component[S, T]` | Pipeline-like components that do work every cycle (caches, DRAM controllers) |
| **Event-driven** | `modeling.EventDrivenComponent[S, T]` | Components that react to messages/events and may be idle for long periods |

Both paradigms use the same port and message infrastructure for communication.
