---
sidebar_position: 3
---

# Event-Driven Components

You have now seen both worlds: the default ticking component from the
first section, and raw events scheduled directly on the engine from the
previous two chapters. Event-driven components sit between them — they
wear the component shape (Spec, State, ports) like the default components,
but wake on demand like raw events instead of every cycle.

That is the right fit when a component is mostly idle, when latencies are
easier to express as absolute times than as cycle counts, or when running
a `Tick` every cycle would be wasted work.

A `modeling.EventDrivenComponent` wakes only when:

- A message arrives on one of its ports, or
- It has scheduled itself to wake at a specific time.

A single `Process` function decides what work is due at each wake-up — it
is the component-shaped counterpart to the handlers from the previous two
chapters.

This chapter walks through `examples/ping`, a ping protocol implemented as
two event-driven components: Agent A sends ping messages, Agent B replies
after a fixed delay.

The source is in `examples/ping/`.

## What You Will Learn

- The `EventDrivenComponent` alternative to the default `Component`.
- Implementing the `EventProcessor` interface.
- Using `ScheduleWakeAt` for latency that does not need a tick counter.
- When to reach for event-driven instead of the default ticking style.

## Spec, State, Comp

Same shape as before:

```go
type Spec struct {
    OutPortBufferSize int `json:"out_port_buffer_size"`
}

type State struct {
    StartTimes       []timing.VTimeInPicoSec
    NextSeqID        int
    PendingResponses []pendingResponse
    ScheduledPings   []scheduledPing
}

type Comp = modeling.EventDrivenComponent[Spec, State, modeling.None]
```

Notice the alias points at `modeling.EventDrivenComponent` rather than
`modeling.Component`. The Spec is simpler — no `Freq`, because there is
no tick.

## The Processor

Instead of one or more `Tick() bool` middlewares, an event-driven
component has a single `Process` function:

```go
type pingProcessor struct{}

func (p *pingProcessor) Process(
    comp *modeling.EventDrivenComponent[Spec, State, modeling.None],
    now timing.VTimeInPicoSec,
) bool {
    progress := false
    state := &comp.State

    progress = p.sendScheduledPings(comp, state, now) || progress
    progress = p.deliverPendingResponses(comp, state, now) || progress
    progress = p.processIncoming(comp, state, now) || progress

    return progress
}
```

`Process` is called whenever the component wakes up. It receives the
current time, looks at its state, and does whatever work is due. The
return value matters only for tracing — there is no "tick again next
cycle" logic to drive.

### Sending scheduled pings

```go
for _, sp := range state.ScheduledPings {
    if sp.SendAt <= now {
        // build pingMsg, send it, record start time
        progress = true
    } else {
        remaining = append(remaining, sp)
        comp.ScheduleWakeAt(sp.SendAt)
    }
}
```

`ScheduleWakeAt(t)` asks the engine to wake this component at simulated
time `t`. If multiple wakeups are requested, only the earliest matters
— the engine deduplicates and replaces.

### Delivering responses after a fixed delay

```go
case *pingReq:
    state.PendingResponses = append(state.PendingResponses,
        pendingResponse{
            DeliverAt: now + 2_000_000_000_000,  // 2 seconds in ps
            Dst:       m.Src,
            OrigMsgID: m.Meta().ID,
            SeqID:     m.SeqID,
        })
    comp.ScheduleWakeAt(now + 2_000_000_000_000)
```

This is the event-driven version of "wait two cycles before responding".
No tick counter, no middleware running every cycle — just schedule a
wakeup two seconds in the future and resume there.

## Wiring

Wiring is almost identical to the default components you have already met:

```go
engine := timing.NewSerialEngine()
registrar := modeling.NewStandaloneRegistrar(engine)

agentA := MakeBuilder().WithRegistrar(registrar).Build("AgentA")
agentB := MakeBuilder().WithRegistrar(registrar).Build("AgentB")

conn := directconnection.MakeBuilder().
    WithRegistrar(registrar).
    Build("Conn")

conn.PlugIn(agentA.GetPortByName("Out"))
conn.PlugIn(agentB.GetPortByName("Out"))

SchedulePing(agentA, 1, agentB.GetPortByName("Out").AsRemote())
SchedulePing(agentA, 3, agentB.GetPortByName("Out").AsRemote())

engine.Run()
```

`SchedulePing(agent, sendAt, dst)` is a helper that appends to
`state.ScheduledPings` and calls `agent.ScheduleWakeAt(sendAt)`. That
single call is enough to start the simulation — there is no `TickLater`
to call because there is no tick.

## Run It

```bash
cd examples/ping
go test -v -run Example
```

Output:

```
Ping 0, 2000000000999 ps
Ping 1, 2000000000997 ps
```

About 2 seconds round-trip plus a tiny delivery overhead.

## When to Reach for This

Reach for an event-driven component when it is mostly idle, when latencies
are easier to express as absolute times than as cycle counts, or when a
per-cycle `Tick` would be doing no work most of the time.

Reach for the default component (the kind you met in the first section)
when work happens every cycle — pipeline stages, controllers, anything
with continuous activity. The two styles share the same Spec/State/Builder
shape, the same ports, and the same messages, so you can freely mix them
in one simulation.

## Key Concepts

- **`EventDrivenComponent` wakes on demand**, not every cycle.
- **`Process(comp, now) bool`** replaces middleware. Do everything in
  one function, branch on what is due.
- **`ScheduleWakeAt(t)`** is the event-driven version of "tick later" —
  the engine wakes you at simulated time `t`.

## Where to Next

You now have every component pattern Akita offers — default ticking
components, raw events for ad-hoc work, and event-driven components for
the idle case. Together with ports and connections and the
hooks-and-tracing tools from the earlier sections, that is the core toolkit
for building and observing an Akita simulation. From here, explore the
ready-made `mem/` and `noc/` packages to compose those pieces into larger
memory hierarchies and networks.
