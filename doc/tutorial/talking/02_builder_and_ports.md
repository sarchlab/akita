---
sidebar_position: 2
---

# The Definition and the Middlewares

The `Definition` ties the Spec, the middlewares, and the ports together.
Every Akita component declares one in the same shape, so once you know one
you know them all.

## Definition

tickingping's middlewares are the fields of its `Middlewares` struct, in
the order they run:

```go
type Middlewares struct {
    // Send sends a due response or the next ping.
    Send *sendMW

    // ReceiveProcess counts down the pings being answered and takes incoming
    // messages.
    ReceiveProcess *receiveProcessMW
}
```

and the package declares its `Definition`, in `definition.go`:

```go
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
    DefaultSpec:    Spec{Freq: 1 * timing.GHz},
    NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
    return Middlewares{
        Send:           &sendMW{comp: c},
        ReceiveProcess: &receiveProcessMW{comp: c},
    }
}
```

Things to notice:

- It has the same shape as the walker's. `Definition.Builder()…Build(name)`
  returns a `*Comp` — the alias from the previous page.
- The component **declares its port** as a field of `Ports` but does not
  create it. The system builder creates the port and passes it to `Build`,
  which binds it to the component; the middlewares then reach it as
  `m.comp.Ports.Out`.
- Middlewares run in the **field order** of `Middlewares`: `Send` first,
  then `ReceiveProcess`.
- `Build` **registers the component and its ports with the simulation**,
  which adds them to checkpointing, tracing, and monitoring.
- The `Definition.Builder()` → `WithX` → `Build(name)` shape is universal
  across Akita components.

The system builder creates the port with `messaging.NewPort` and passes it
through `WithPorts`:

```go
outA := messaging.NewPort(nil, 16, 16, "AgentA.Out")

agentA := Definition.Builder().
    WithSimulation(sim).
    WithSpec(specA).
    WithPorts(Ports{Out: outA}).
    Build("AgentA")
```

`NewPort(nil, 16, 16, "AgentA.Out")` creates a port with no owner yet, room
for 16 incoming and 16 outgoing messages, and the name
`<instance>.<field>`. `Build` sets the owner, and it checks the name and
panics on a mismatch, so a typo fails fast. Every port in `Ports` must be
given, and none is added after `Build`. This keeps the component agnostic
to how its port is built (buffer sizes, instrumentation) while the
component still owns which ports exist. The next page wires two agents
together this way.

## Why a Component Package?

In *Create a Component* the walker lived in `package main`, next to the
code that built it. tickingping is its own package, and that is the
convention for every real component: **one component type per package**.
(A component type's name is its package's import path.)

A package of its own buys three things:

- **Correct assembly, every time.** The package declares the parts once —
  the five structs, the middlewares, the `Definition` — and `Build`
  assembles them the same way for every instance. A caller cannot forget a
  middleware or run them in the wrong order; `Build(name)` always returns a
  fully wired `*Comp`.
- **Easy instances.** `Definition.Builder().WithSpec(spec).WithPorts(ports).Build(name)`
  stamps out an agent on demand — we build two, AgentA and AgentB — and
  `Definition.DefaultSpec` gives a base configuration to tweak.
- **A clean surface.** Callers see only what they must supply — the Spec,
  the Resources, the Ports — and the `Definition`; the middlewares are
  unexported. Tooling reads the same `Definition` without running the code,
  and each component package keeps the two views in agreement with a
  one-line test, `modelingtest.CheckTicking(t, Definition)`.

Rule of thumb: keep a trivial, single-use component next to `main`; give it
a package once it owns ports, has more than one middleware, or gets
instantiated more than once — which is to say, nearly every real component.

## Middleware: Where Behaviour Lives

A component has one or more middlewares. Each tick, the component calls
every middleware's `Handle` method in field order. If **any** middleware
returns true, the component ticks again on the next cycle. tickingping uses
two.

### `sendMW` — pushes work out

```go
func (m *sendMW) Handle(_ timing.Event) bool {
    madeProgress := false

    madeProgress = m.sendRsp() || madeProgress
    madeProgress = m.sendPing() || madeProgress

    return madeProgress
}
```

`sendRsp` checks whether the oldest ping being answered has finished its
countdown and, if so, builds a `pingRsp` and sends it. `sendPing` checks
whether more pings are due and, if so, builds a `pingReq`. Messages are
values, so the literal has no `&`:

```go
func (m *sendMW) sendPing() bool {
    state := &m.comp.State
    spec := m.comp.Spec()
    out := m.comp.Ports.Out

    if state.NextSeqID >= spec.NumPings {
        return false
    }

    if !out.CanSend() {
        return false
    }

    out.Send(pingReq{
        MsgMeta: messaging.MsgMeta{
            ID:  m.comp.NewID(),
            Src: out.AsRemote(),
            Dst: spec.PingDst,
        },
        SeqID: state.NextSeqID,
    })

    state.StartTimes = append(state.StartTimes, uint64(m.comp.CurrentTime()))
    state.NextSeqID++

    return true
}
```

`Send` takes the message by value and returns nothing, so you guard it
with `CanSend()` instead of checking a return error. If the outgoing
port's buffer is full, `CanSend()` is false and we return false — no
progress this cycle. The component wakes again when the port has room.

### `receiveProcessMW` — pulls work in

```go
func (m *receiveProcessMW) Handle(_ timing.Event) bool {
    madeProgress := false

    madeProgress = m.countDown() || madeProgress
    madeProgress = m.processInput() || madeProgress

    return madeProgress
}
```

`countDown` decrements the cycle counter of every ping being answered —
this is the "fake latency" that makes the response take two cycles.
`processInput` takes the next incoming message; a `pingReq` starts a new
transaction, and a `pingRsp` prints the round-trip time.

```go
msgI, ok := m.comp.Ports.Out.RetrieveIncoming()
if !ok {
    return false
}

switch msg := msgI.(type) {
case pingReq:
    m.processPingReq(msg)
case pingRsp:
    m.processPingRsp(msg)
default:
    panic("unknown message type")
}

return true
```

Because messages are values, the type switch matches on value cases
(`pingReq`, `pingRsp`) — not pointer cases. `RetrieveIncoming` consumes the
message and returns it as a `messaging.Msg` interface value with a presence
boolean.

tickingping can always handle a message, so it retrieves at once. A
component that sometimes cannot — its output port is full, or it has no
free resource — calls `PeekIncoming` first. Peek does not consume the
message: you can look at it, decide you cannot process it, and leave it for
next cycle. `RetrieveIncoming` is the commit. The client and server in
*Tracing Requests* work that way.

## Where to Next

The component is complete. The last page is the system builder: it wires
two instances together with a connection and runs the simulation.
