---
sidebar_position: 3
---

# Wiring and Running

Two agents, a connection between them, some initial ping traffic, and the
engine. This is the system builder, where the components from the previous
pages actually talk.

## Wiring It All Together

```go
engine := timing.NewSerialEngine()
sim := modeling.NewStandaloneSimulation(engine)

// Create the ports first, so AgentA's Spec can name AgentB's port.
outA := messaging.NewPort("AgentA.Out", 16, 16)
outB := messaging.NewPort("AgentB.Out", 16, 16)

specA := Definition.DefaultSpec
specA.Freq = 1 * timing.Hz
specA.PingDst = outB.AsRemote()
specA.NumPings = 2

specB := Definition.DefaultSpec
specB.Freq = 1 * timing.Hz

agentA := Definition.Builder().
    WithSimulation(sim).
    WithSpec(specA).
    WithPorts(Ports{Out: outA}).
    Build("AgentA")

agentB := Definition.Builder().
    WithSimulation(sim).
    WithSpec(specB).
    WithPorts(Ports{Out: outB}).
    Build("AgentB")

conn := directconnection.MakeBuilder().
    WithSimulation(sim).
    Build("Conn")

conn.PlugIn(agentA.Ports.Out)
conn.PlugIn(agentB.Ports.Out)

// AgentA sends pings on its own, so start it; AgentB wakes when a ping
// arrives.
agentA.TickLater()

err := engine.Run()
```

This code is the package's `Example` test, so it writes `Definition` and
`Ports`; a system builder outside the package writes
`tickingping.Definition` and `tickingping.Ports`.

Step by step:

1. **Engine.** A serial engine for deterministic runs.
2. **Simulation.** `NewStandaloneSimulation` supplies a lightweight context
   shared by the agents and connection, including their ID counter.
   `sim.MakeBuilder().Build()` adds recording and inventory; attach a monitor with `WithMonitor`.
3. **Create ports.** The system builder creates both ports with
   `messaging.NewPort`, choosing the buffer sizes (16 incoming, 16 outgoing)
   and naming each `<instance>.<field>`. Creating them before the agents
   lets AgentA's Spec name AgentB's port.
4. **Choose specs.** Both agents start from `Definition.DefaultSpec` and
   slow the clock to 1 Hz. AgentA is also told whom to ping (`PingDst`, the
   remote address of B's port) and how many times (`NumPings = 2`). AgentB
   keeps `NumPings` at zero, so it only responds.
5. **Build agents.** Two instances of the same component type, named
   `AgentA` and `AgentB`, each given its own port with `WithPorts`. `Build`
   binds each port to its agent and registers both with the simulation.
6. **Build connection.** A `directconnection` — zero-latency, ideal for
   simple topologies.
7. **Plug ports.** Each agent's `Out` port, reached as `agent.Ports.Out`,
   goes into the connection. Now any port plugged into this connection can
   reach any other.
8. **Kick it off.** `TickLater` schedules Agent A to tick on the next
   cycle. Agent B needs no kick: it wakes when a ping arrives on its port.
9. **Run.** The engine fires ticks until no component has progress to make.
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

- **A component = five structs + a `Definition`.** The same shape as the
  single component, now with a port and a second middleware.
- **The system builder owns the wiring.** It creates every port with
  `messaging.NewPort`, passes the ports to `Build` with `WithPorts`, and
  plugs them into connections.
- **Ports buffer messages.** Messages are value types: construct them with
  no `&`, check `CanSend()` before `Send` because the outgoing buffer can
  be full, and `Peek` lets you look at incoming messages without consuming.
- **Connections move messages.** Plug ports into a `directconnection` and
  any plugged port can reach any other.
- **The builder pattern is universal.** Components are built with
  `Definition.Builder().WithX().Build(name)`, and connections with
  `directconnection.MakeBuilder().WithX().Build(name)`.

## Where to Next

You can build a working simulation with everything in these two sections.
The next section, **Getting Information from a Simulation**, shows how to
observe one: attach
callbacks that log events and messages, and measure how long work takes —
all without changing the components you just wrote.
