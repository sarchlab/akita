# examples — Akita Example Programs

This directory contains example programs demonstrating how to use the Akita
simulation framework. They progress from minimal event scheduling to full
component communication patterns.

## Examples

### 01_print_event — Basic Event Scheduling

The simplest Akita program. Creates an engine, registers an event handler,
schedules one event, and runs the simulation.

**Key concepts**: `timing.Engine`, `timing.Event`, `timing.Handler`, event
scheduling.

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

A single ticking component, no ports. Takes one ±1 step per cycle
until it drifts to ±10 from the origin, then prints the final position,
step count, and simulated time. Smallest useful Akita simulation.

**Key concepts**: the five structs of a component (`Spec`, `State`,
`Resources`, `Ports`, `Middlewares`), the package-level `Definition`, the
`type Comp = ticking.Component[...]` alias, the
`Handle(e timing.Event) bool` middleware contract, `TickLater` to start the
loop, a seeded RNG supplied by the system builder as a `Resources` field.

```bash
cd 03_random_walk && go run main.go
```

### tickingping — Tick-Based Component Communication

Two ticking components (`ticking.Component`) exchange ping/response
messages. Each has two middlewares, run in order every tick:

- **Send** — sends a due response, or the next of `Spec.NumPings` pings to
  `Spec.PingDst`.
- **ReceiveProcess** — counts down the pings being answered and takes
  incoming messages.

**Key concepts**: a component package (five structs plus `Definition`),
ports created by the system builder with `messaging.NewPort` and passed to
`Build`, a `Spec` field naming a peer's port, `modelingtest.CheckTicking`.

```bash
cd tickingping && go test -v -run Example
```

### ping — Wakeup Component Communication

Two wakeup components (`wakeup.Component`) exchange ping/response messages.
A wakeup component runs only when a port has activity or at a time it asked
for with `WakeAt`, so it costs nothing while it waits out the delay before
each response. A single middleware sends scheduled pings, answers incoming
pings after a delay, and reports the round-trip times.

**Key concepts**: the wakeup model, `WakeAt`, a documented package function
(`SchedulePing`) as the way to hand a component work from outside.

```bash
cd ping && go test -v -run Example
```

### checkpointdemo — Checkpoint and Resume

A ticking worker processes two batches of work with an idle gap between
them. The `save` mode checkpoints in the gap; the `load` mode resumes from
that checkpoint, and both finish with the same counts, checksum, and time.

**Key concepts**: `Simulation.SaveCheckpoint`/`LoadCheckpoint`, `State` as
the component's checkpointed data, a package function (`addWork`) as the
component's entry point for new work.

```bash
go run ./examples/checkpointdemo -mode save -ckpt /tmp/ck.tar.gz
go run ./examples/checkpointdemo -mode load -ckpt /tmp/ck.tar.gz
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
`AddTaskTag`/`AddMilestone` via the embedded `tracing.NopTracer`) and reports
the longest job. Attached the same way as a built-in tracer, with
`tracing.CollectTrace`.

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
from the same run. Each component type is in its own package (`client`,
`server`); `main.go` wires them together.

**Key concepts**: nested `req_out`/`req_in` tasks, the four `TraceReq*`
helpers, selecting tasks by `Kind` with a `TaskFilter`.

```bash
cd reqtracing && go run main.go
```

### tasktree — Chaining Tasks Across a Hierarchy

A request travels a memory hierarchy (`Client → L1 → L2 → Memory`); each cache
misses and forwards downward, parenting the downstream task to the one it is
handling with `tracing.MsgIDAtReceiver`. A custom tracer attached to every
component prints the resulting task tree. Each component type is in its own
package (`client`, `cache`, `memory`); `main.go` builds the hierarchy bottom-up
so each level's `Spec` can name the port of the level below.

**Key concepts**: `tracing.MsgIDAtReceiver`, `parentID` chaining, task trees
across components, attaching one tracer to many domains.

```bash
cd tasktree && go run main.go
```

## Choosing a Component Model

| Model | Component Type | When to Use | Example |
|---|---|---|---|
| **Ticking** | `ticking.Component` | Work advances cycle by cycle (pipelines, caches, DRAM controllers); the default | `tickingping` |
| **Wakeup** | `wakeup.Component` | Idle most of the time and knows when it next has work | `ping` |
| **Event** | `event.Component` | A set of distinct happenings, each with its own data and time | the `modeling/event` package example |

All models share the five structs, the `Definition`, and the port and
message infrastructure; they differ only in which events reach the
middlewares. See the "Component Models" section of `modeling/README.md`.
