---
sidebar_position: 3
---

# Wiring and Running

Two agents, a connection between them, some initial ping traffic, and the
engine. This is where the components from the previous pages actually talk.

## Wiring It All Together

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
6. **Set initial state.** Tell Agent A who to ping (`PingDst`, the remote
   address of B's port) and how many times (`NumPingNeedToSend = 2`).
7. **Kick it off.** `TickLater` schedules Agent A to tick on the next
   cycle.
8. **Run.** The engine fires ticks until no component has progress to make.
   Agent A sends pings; Agent B counts down and replies; Agent A sees the
   responses and prints durations.

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

- **A component = Spec + State + Middleware + Builder.** The same shape as
  the single component, now with ports added.
- **Ports buffer messages.** Messages are value types: construct them with
  no `&`, check `CanSend()` before `Send` because the outgoing buffer can
  be full, and `Peek` lets you look at incoming messages without consuming.
- **Connections move messages.** Plug ports into a `directconnection` and
  any plugged port can reach any other.
- **The builder pattern is universal.** Every component and connection uses
  the same `MakeBuilder().WithX().Build(name)` shape.

## Where to Next

You can build a working simulation with everything in these two sections.
The next section, **Getting Information from a Simulation**, shows how to
observe one: attach
callbacks that log events and messages, and measure how long work takes —
all without changing the components you just wrote.
