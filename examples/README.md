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

### 03_random_walk — Single-Component Random Walk

A single tick-based component, no ports. Takes one ±1 step per cycle
until it drifts to ±10 from the origin, then prints the final position,
step count, and simulated time. Smallest useful Akita simulation.

**Key concepts**: `modeling.Component`, the
`type Comp = modeling.Component[Spec, State, modeling.None]` alias,
`Spec`/`State` separation, the `Tick() bool` middleware contract,
`TickLater` to start the loop, middleware holding auxiliary state (a seeded
RNG) outside Spec/State.

```bash
cd 03_random_walk && go run main.go
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

### hooks — Observing a Simulation with Hooks

Two ticking agents exchange a ping/response, observed entirely from the
outside. An engine hook logs every event and a port hook logs every message;
the agents themselves print nothing. Shows the `hooking` API
(`Hook`, `HookCtx`, `AcceptHook`) and the engine/port hook positions.

**Key concepts**: `hooking.Hook`, `HookCtx`, `AcceptHook`,
`timing.HookPosBeforeEvent`, `messaging.HookPosPortMsgSend`/`Recvd`.

```bash
cd hooks && go run main.go
```

### customhook — Defining Your Own Hook Point

A random walker defines its own `HookPos` and fires it with `InvokeHook` on
every step, exposing internal behavior the built-in hook points cannot see.
An external hook logs each step; the walker itself prints nothing.

**Key concepts**: declaring a `hooking.HookPos`, `InvokeHook`, a component as
`Hookable`, payload via `HookCtx.Item`.

```bash
cd customhook && go run main.go
```

### tracing — Measuring Work with Tracing Tasks

A single worker component wraps each job in a tracing task
(`tracing.StartTask` / `EndTask`). A `BusyTimeTracer` and an
`AverageTimeTracer`, attached with `tracing.CollectTrace`, report how long
the worker was busy and how long an average job took.

**Key concepts**: tracing tasks, `tracing.CollectTrace`, `BusyTimeTracer`,
`AverageTimeTracer`, `TaskFilter` — tracing built on hooks.

```bash
cd tracing && go run main.go
```

### customtracer — Writing Your Own Tracer

A worker plus a hand-written `maxDurationTracer` that implements the
`tracing.Tracer` interface (real `StartTask`/`EndTask`, no-op
`StepTask`/`AddMilestone`) and reports the longest job. Attached the same way
as a built-in tracer, with `tracing.CollectTrace`.

**Key concepts**: the `tracing.Tracer` interface, holding state across
`StartTask`/`EndTask`, reading a `timing.TimeTeller`.

```bash
cd customtracer && go run main.go
```

### reqtracing — Tracing the Request Lifecycle

A client and server exchange request/response over a direct connection,
annotated with the four request helpers (`TraceReqInitiate`, `TraceReqReceive`,
`TraceReqComplete`, `TraceReqFinalize`). Two `AverageTimeTracer`s — filtering
`req_out` and `req_in` — report round-trip latency and server handling time
from the same run.

**Key concepts**: nested `req_out`/`req_in` tasks, the four `TraceReq*`
helpers, selecting tasks by `Kind` with a `TaskFilter`.

```bash
cd reqtracing && go run main.go
```

### tasktree — Chaining Tasks Across a Hierarchy

A request travels a memory hierarchy (`Client → L1 → L2 → Memory`); each cache
misses and forwards downward, parenting the downstream task to the one it is
handling with `tracing.MsgIDAtReceiver`. A custom tracer attached to every
component prints the resulting task tree.

**Key concepts**: `tracing.MsgIDAtReceiver`, `parentID` chaining, task trees
across components, attaching one tracer to many domains.

```bash
cd tasktree && go run main.go
```

## Choosing a Paradigm

| Paradigm | Component Type | When to Use |
|---|---|---|
| **Ticking** | `modeling.Component[S, T]` | Pipeline-like components that do work every cycle (caches, DRAM controllers) |
| **Event-driven** | `modeling.EventDrivenComponent[S, T]` | Components that react to messages/events and may be idle for long periods |

Both paradigms use the same port and message infrastructure for communication.
