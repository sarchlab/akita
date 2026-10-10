---
sidebar_position: 3
---

# Wakeup and Event Components

You have now seen both worlds: ticking components from the first section,
and raw events scheduled directly on the engine from the previous two
chapters. Akita has three component models, and two of them sit between
those worlds: they wear the component shape (five structs and a
`Definition`) but have no clock.

| Model | Runs when | Its middlewares |
|---|---|---|
| `modeling/ticking` | every cycle while it makes progress | do one cycle of work |
| `modeling/wakeup` | a port has activity, or at a time it asked for | poll the State and ports for work that is ready |
| `modeling/event` | an event arrives: port activity, or an event it scheduled | react to specific event types and their data |

In all three, a middleware is the same thing, an event handler:

```go
type Middleware interface {
    Handle(e timing.Event) bool
}
```

The component passes every event it receives to its middlewares in field
order. What differs between the models is only which events arrive.

## What You Will Learn

- Writing a wakeup component, using `examples/ping`.
- Using `WakeAt` for latency that does not need a tick counter.
- Writing an event component that schedules events carrying data.
- Choosing between ticking, wakeup, and event.

## A Wakeup Component: Ping

`examples/ping` implements a ping protocol with two wakeup components:
Agent A sends pings, and Agent B replies after a fixed delay.

### The five structs

```go
type Spec struct{}

type State struct {
    StartTimes       []timing.VTimeInPicoSec
    NextSeqID        int
    PendingResponses []pendingResponse
    ScheduledPings   []scheduledPing
}

type Ports struct {
    Out messaging.Port
}

type Middlewares struct {
    Ping *pingMW
}

type Comp = wakeup.Component[Spec, State, modeling.None, Ports, Middlewares]

var Definition = wakeup.Definition[Spec, State, modeling.None, Ports, Middlewares]{
    DefaultSpec:    Spec{},
    NewMiddlewares: newMiddlewares,
}
```

The Spec is empty: there is no `Freq`, because there is no clock.

### The middleware

```go
func (m *pingMW) Handle(e timing.Event) bool {
    now := e.Time()
    progress := false

    progress = m.sendScheduledPings(now) || progress
    progress = m.deliverPendingResponses(now) || progress
    progress = m.processIncoming(now) || progress

    return progress
}
```

`Handle` runs on every wakeup. The event is a `wakeup.Event`, which
carries nothing but its time: the middleware looks at the State and the
port and does whatever work is due. If it made progress, the component
wakes again at once, so it keeps running until nothing is left to do.

Work that is not due yet asks for a wakeup at its time:

```go
for _, sp := range state.ScheduledPings {
    if sp.SendAt > now {
        remaining = append(remaining, sp)
        m.comp.WakeAt(sp.SendAt)
        continue
    }
    // build the ping, send it, record its start time
}
```

`WakeAt(t)` asks the engine to wake the component at simulated time `t`.
Asking for a wakeup at or after one already pending does nothing, so a
middleware asks again, each time it wakes, for the work still waiting.

Responses use the same trick: a ping schedules its response two seconds
later, with no tick counter.

```go
case pingReq:
    state.PendingResponses = append(state.PendingResponses,
        pendingResponse{DeliverAt: now + 2_000_000_000_000, ...})
    m.comp.WakeAt(now + 2_000_000_000_000)
```

### Wiring

```go
engine := timing.NewSerialEngine()
sim := modeling.NewStandaloneSimulation(engine)

agentA := ping.Definition.Builder().
    WithSimulation(sim).
    Build("AgentA")
agentA.BindPort("Out", twowaybuffered.NewPort(16, 16))
agentB := ping.Definition.Builder().
    WithSimulation(sim).
    Build("AgentB")
agentB.BindPort("Out", twowaybuffered.NewPort(16, 16))

conn := direct.NewConnection("Conn", sim, timing.GHz)
conn.BindPort(agentA.Ports.Out)
conn.BindPort(agentB.Ports.Out)

if err := sim.Initialize(); err != nil { panic(err) }

ping.SchedulePing(agentA, 1, agentB.Ports.Out.AsRemote())
ping.SchedulePing(agentA, 3, agentB.Ports.Out.AsRemote())

engine.Run()
```

The system builder creates each port, choosing its buffer sizes, and passes
them to `component.BindPort` after Build, then calls `Initialize`. `SchedulePing`
records a ping in the State and calls `WakeAt(sendAt)`; that is enough to
start the simulation.

### Run it

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

## An Event Component: A Delay Line

An event component has no clock either, but instead of polling, its
middlewares react to events that carry data. Port activity arrives as
`event.Recv` and `event.PortFree`, each naming its port, and the component
schedules its own events with `Schedule`. The example in `modeling/event` is
a delay line that completes every request a fixed latency after it arrives:

```go
type doneEvent struct {
    timing.EventBase
    ReqID uint64 `json:"req_id"`
}

func init() { timing.RegisterEvent(doneEvent{}) }

func (m *delayMW) Handle(e timing.Event) bool {
    switch e := e.(type) {
    case event.Recv:
        for {
            msg, ok := m.comp.Ports.In.RetrieveIncoming()
            if !ok {
                break
            }
            m.comp.Schedule(doneEvent{
                EventBase: m.comp.MakeEventBase(e.Time() + m.comp.Spec.Latency),
                ReqID:     msg.ID,
            })
        }
    case doneEvent:
        m.comp.State.Done = append(m.comp.State.Done,
            done{ID: e.ReqID, At: e.Time()})
    default:
        return false
    }
    return true
}
```

The request's ID travels in the event itself, so there is no list of
pending work to scan. Register every event type the component schedules
with `timing.RegisterEvent`, so a checkpoint can hold it while it is
pending.

## Choosing a Model

- **Ticking** when work happens cycle by cycle: pipeline stages, caches,
  controllers, anything with continuous activity. It is the default.
- **Wakeup** when the component is mostly idle and knows when it next has
  work, and a middleware can find that work by looking at the State.
- **Event** when the behavior is a set of distinct happenings, each with its
  own data and time, like requests that complete after computed latencies.

All three share the five structs, the ports, and the messages, so you can
freely mix them in one simulation.

## Key Concepts

- **A middleware is `Handle(e timing.Event) bool`** in every model; the
  models differ only in which events arrive.
- **A wakeup component polls** when woken and asks for future wakeups with
  `WakeAt(t)`.
- **An event component reacts** to `Recv`, `PortFree`, and the events it
  schedules for itself with `Schedule`.

## Where to Next

You now have every component pattern Akita offers — ticking components,
raw events for ad-hoc work, and wakeup and event components for the idle
and event-shaped cases. Together with ports and connections and the
hooks-and-tracing tools from the earlier sections, that is the core toolkit
for building and observing an Akita simulation. From here, explore the
ready-made `mem/` and `noc/` packages to compose those pieces into larger
memory hierarchies and networks.
