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
whether more pings are queued and, if so, builds a `pingReq`. The actual
send:

```go
err := outPort(m.comp).Send(pingMsg)
if err != nil {
    return false
}
```

If the outgoing port's buffer is full, `Send` returns an error and we
return false — no progress this cycle. The engine will retry next tick.

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
case *pingReq:
    m.processingPingReq(msg)
case *pingRsp:
    m.processingPingRsp(msg)
}
```

Note `Peek` then `Retrieve`: peek does not consume the message — you can
look at it, decide you cannot process it (port full, no resource), and
leave it for next cycle. `Retrieve` is the commit.

## Where to Next

The component is complete. The last page wires two of them together with a
connection and runs the simulation.
