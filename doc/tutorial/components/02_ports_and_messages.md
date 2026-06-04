---
sidebar_position: 2
---

# Ports and Messages

The previous chapter showed the minimal component — one component, no
ports, one middleware. This chapter shows the full pattern: **ports** for
inter-component messages, **multiple middlewares** in one component, and
the per-package **builder** convention used throughout Akita.

The example is `examples/tickingping`, a two-component setup where Agent A
sends ping messages to Agent B and Agent B replies after a fake latency.
Both agents use the same component type.

The source is in `examples/tickingping/`.

## What You Will Learn

- The five parts of a component: Spec, State, middleware, builder,
  registration.
- What "ticking" means and how middleware advances state each cycle.
- How ports and `directconnection` move messages between components.

## The Five Parts of a Component

| Part | Where | Role |
|---|---|---|
| **Spec** | `comp.go` | Immutable configuration. JSON-serializable primitives. |
| **State** | `comp.go` | Mutable runtime data. JSON-serializable. |
| **Middleware** | `sendmw.go`, `receiveprocessmw.go` | A struct with `Tick() bool`. Multiple per component. |
| **Builder** | `builder.go` | Wires everything together with the `MakeBuilder().WithX().Build(name)` pattern. |
| **Component alias** | `comp.go` | A type alias for `modeling.Component[Spec, State, modeling.None]`. |

### Spec

```go
type Spec struct {
    Freq              timing.Freq `json:"freq"`
    OutPortBufferSize int         `json:"out_port_buffer_size"`
}
```

Spec captures things that do not change at runtime: clock frequency, port
buffer sizes, latencies. Use primitive types and tag every field for JSON
so the spec is checkpoint-friendly.

### State

```go
type State struct {
    StartTimes          []uint64               `json:"start_times"`
    NextSeqID           int                    `json:"next_seq_id"`
    NumPingNeedToSend   int                    `json:"num_ping_need_to_send"`
    PingDst             messaging.RemotePort   `json:"ping_dst"`
    CurrentTransactions []pingTransactionState `json:"current_transactions"`
}
```

State holds everything the component remembers between ticks: counters,
in-flight transactions, pending work. Same rule: JSON-serializable, no
pointers or interfaces.

### The component alias

```go
type Comp = modeling.Component[Spec, State, modeling.None]
```

`modeling.Component` is generic over Spec, State, and (a rarely used)
auxiliary type. Aliasing it keeps long type names short.

## Messages

Two message types, defined alongside the component:

```go
type pingReq struct {
    messaging.MsgMeta
    SeqID int
}

type pingRsp struct {
    messaging.MsgMeta
    SeqID int
}
```

`messaging.MsgMeta` is embedded in every message and carries routing
metadata (source, destination, ID, response-to-ID). The `SeqID` field is
the payload you actually care about.

## Middleware: Where Behaviour Lives

A component has one or more middlewares. Each tick, the engine calls
every middleware's `Tick() bool` method in registration order. If
**any** middleware returns true, the component is rescheduled for the
next tick.

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
whether more pings are queued to be sent and, if so, builds a `pingReq`.

The actual send:

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
`processInput` peeks at any incoming message; if there is a `pingReq`,
it adds a new transaction; if there is a `pingRsp`, it prints the
duration and acknowledges receipt.

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

## Builder

The builder ties Spec, middleware, and ports together. Every Akita
builder follows the same shape:

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

- The builder **creates the port internally** — callers do not pass ports
  in. They are accessed later through `comp.GetPortByName("Out")`.
- Middlewares are added in order; the first one added runs first.
- The component is **registered with the registrar**, which integrates it
  with the engine and the broader simulation.
- The `MakeBuilder` → `WithX` → `Build(name)` shape is universal across
  Akita components and connections. Once you know one builder, you know
  them all.

## Wiring It All Together

The example test builds two agents, connects them, kicks off ping
traffic, and runs the engine:

```go
engine := timing.NewSerialEngine()
registrar := modeling.NewStandaloneRegistrar(engine)

agentSpec := DefaultSpec()
agentSpec.Freq = 1 * timing.Hz

agentA := MakeBuilder().
    WithRegistrar(registrar).
    WithSpec(agentSpec).
    Build("AgentA")

agentB := MakeBuilder().
    WithRegistrar(registrar).
    WithSpec(agentSpec).
    Build("AgentB")

conn := directconnection.MakeBuilder().
    WithRegistrar(registrar).
    Build("Conn")

conn.PlugIn(agentA.GetPortByName("Out"))
conn.PlugIn(agentB.GetPortByName("Out"))

state := agentA.State
state.PingDst = agentB.GetPortByName("Out").AsRemote()
state.NumPingNeedToSend = 2
agentA.State = state

agentA.TickLater()

err := engine.Run()
```

Step by step:

1. **Engine.** A serial engine for deterministic runs.
2. **Registrar.** `NewStandaloneRegistrar` is the test-friendly registrar.
   In a real simulation, `simulation.MakeBuilder().Build()` gives you one
   as part of a richer setup.
3. **Build agents.** Two instances of the same component, named `AgentA`
   and `AgentB`.
4. **Build connection.** A `directconnection` — zero-latency, ideal for
   simple topologies.
5. **Plug ports.** Each agent's `Out` port goes into the connection. Now
   any port plugged into this connection can reach any other.
6. **Set initial state.** Tell Agent A who to ping (`PingDst`) and how
   many times (`NumPingNeedToSend = 2`).
7. **Kick it off.** `TickLater` schedules Agent A to tick on the next
   cycle.
8. **Run.** The engine fires ticks until no component has progress to
   make. Agent A sends pings; Agent B counts down and replies; Agent A
   sees the responses and prints durations.

## Run It

```bash
cd examples/tickingping
go test -v -run Example
```

Output:

```
Ping 0, 5000000000000 ps
Ping 1, 5000000000000 ps
```

5 seconds round-trip at 1 Hz, which checks out: one cycle to send, two
cycles to count down, one cycle to send the response, and a delivery
cycle.

## Key Concepts

- **A component = Spec + State + Middleware + Builder.** Memorise this
  shape; every component in Akita follows it.
- **Spec is immutable, State is mutable.** Both are
  JSON-serializable.
- **Middleware is called every tick.** Return true if you made progress;
  the engine will tick again next cycle.
- **Ports buffer messages.** `Send` may fail because the outgoing buffer
  is full; `Peek` lets you look without consuming.
- **The builder pattern is universal.** Every component and connection
  uses the same `MakeBuilder().WithX().Build(name)` shape.

## Where to Next

You can build a working simulation with everything in this section. The
next section opens the layer underneath: the events and handlers that the
engine itself uses to schedule and dispatch work — and, at the end, an
event-driven component variant for the cases where ticking every cycle is
wasteful.
