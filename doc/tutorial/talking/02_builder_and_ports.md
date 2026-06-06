---
sidebar_position: 2
---

# The Builder and the Middlewares

The builder ties Spec, middleware, and ports together. Every Akita builder
follows the same shape, so once you know one you know them all.

## Builder

```go
type Builder struct {
    spec      Spec
    registrar modeling.Registrar
}

func MakeBuilder() Builder { return Builder{spec: defaultSpec} }

func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
    b.registrar = reg
    return b
}

func (b Builder) WithSpec(spec Spec) Builder {
    b.spec = spec
    return b
}

func (b Builder) Build(name string) *Comp {
    comp := modeling.NewBuilder[Spec, State, modeling.None]().
        WithEngine(b.registrar.GetEngine()).
        WithFreq(b.spec.Freq).
        WithSpec(b.spec).
        Build(name)
    comp.State = State{}

    comp.AddMiddleware(&sendMW{comp: comp})
    comp.AddMiddleware(&receiveProcessMW{comp: comp})

    outPort := messaging.NewPort(
        comp, b.spec.OutPortBufferSize, b.spec.OutPortBufferSize, name+".Out")
    comp.AddPort("Out", outPort)

    b.registrar.RegisterComponent(comp)

    return comp
}
```

Things to notice:

- The builder returns a `*Comp` — the same alias from the previous page.
- The builder **creates the port internally** — callers do not pass ports
  in. They access it later through `comp.GetPortByName("Out")`.
- Middlewares are added in order; the first one added runs first.
- The component is **registered with the registrar**, which integrates it
  with the engine and the broader simulation.
- The `MakeBuilder` → `WithX` → `Build(name)` shape is universal across
  Akita components and connections.

> **Convention note:** Akita is moving port creation out of components. New
> components declare their ports in `Build` with `DeclarePort` and have setup
> code assign the instances afterward with `AssignPort` (see `idealmemcontroller`
> and the `messaging` package README). `tickingping` still creates its `Out`
> port inside `Build` as shown here; the externalized model is the direction
> going forward.

## Why a Custom Builder?

In *Create a Component* the walker had no ports and a single middleware, so we
built it inline with `modeling.NewBuilder` right in `main`. Assembling this
component is more work: create the `Out` port at the configured buffer size,
add two middlewares in the right order, and register with the registrar.
Repeating all of that at every call site — and we build two agents — would be
verbose and easy to get wrong.

Wrapping construction in a per-package builder buys three things:

- **Correct assembly, every time.** All the steps live inside `Build`, so a
  caller cannot forget to add a middleware or register the component;
  `Build(name)` always returns a fully wired `*Comp`.
- **Easy instances.** `MakeBuilder().WithSpec(spec).Build(name)` stamps out an
  agent on demand — we call it twice, for AgentA and AgentB — and
  `DefaultSpec()` gives a base configuration to tweak.
- **A clean surface.** The `modeling.NewBuilder` generics and the ports are
  written once, inside `Build`; callers only ever see
  `MakeBuilder → WithX → Build`. This is the per-package builder that the
  *Create a Component* section's builder-pattern callout pointed ahead to.

Rule of thumb: build inline when a component is trivial; give it a builder
once it owns ports, has more than one middleware, or gets instantiated more
than once — which is to say, nearly every real component.

## Middleware: Where Behaviour Lives

A component has one or more middlewares. Each tick, the engine calls every
middleware's `Tick() bool` method in registration order. If **any**
middleware returns true, the component is rescheduled for the next tick.
tickingping uses two.

### `sendMW` — pushes work out

```go
func (m *sendMW) Tick() bool {
    madeProgress := false
    madeProgress = m.sendRsp() || madeProgress
    madeProgress = m.sendPing() || madeProgress
    return madeProgress
}
```

`sendRsp` checks whether any in-flight transaction has finished its
countdown and, if so, builds a `pingRsp` and sends it. `sendPing` checks
whether more pings are queued and, if so, builds a `pingReq`. Messages are
values, so the literal has no `&`:

```go
pingMsg := pingReq{
    MsgMeta: messaging.MsgMeta{
        ID:  timing.GetIDGenerator().Generate(),
        Src: outPort(m.comp).AsRemote(),
        Dst: state.PingDst,
    },
    SeqID: state.NextSeqID,
}

if !outPort(m.comp).CanSend() {
    return false
}

outPort(m.comp).Send(pingMsg)
```

`Send` takes the message by value and returns nothing, so you guard it
with `CanSend()` instead of checking a return error. If the outgoing
port's buffer is full, `CanSend()` is false and we return false — no
progress this cycle. The engine will retry next tick.

### `receiveProcessMW` — pulls work in

```go
func (m *receiveProcessMW) Tick() bool {
    madeProgress := false
    madeProgress = m.countDown() || madeProgress
    madeProgress = m.processInput() || madeProgress
    return madeProgress
}
```

`countDown` decrements the cycle counter of any in-flight transaction —
this is the "fake latency" that makes the response take two cycles.
`processInput` peeks at any incoming message; if there is a `pingReq` it
adds a new transaction, and if there is a `pingRsp` it prints the duration
and acknowledges receipt.

```go
msgI := outPort(m.comp).PeekIncoming()
if msgI == nil {
    return false
}

switch msg := msgI.(type) {
case pingReq:
    m.processingPingReq(msg)
case pingRsp:
    m.processingPingRsp(msg)
default:
    panic("unknown message type")
}
```

Because messages are values, the type switch matches on value cases
(`pingReq`, `pingRsp`) — not pointer cases. `PeekIncoming` returns the
message as a `messaging.Msg` interface value; the handlers then call
`RetrieveIncoming()` to consume it.

Note `Peek` then `Retrieve`: peek does not consume the message — you can
look at it, decide you cannot process it (port full, no resource), and
leave it for next cycle. `Retrieve` is the commit.

## Where to Next

The component is complete. The last page wires two of them together with a
connection and runs the simulation.
