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

// Choose buffer sizes; binding below assigns each port its name.
outA := twowaybuffered.NewPort(16, 16)
outB := twowaybuffered.NewPort(16, 16)

specA := Definition.DefaultSpec
specA.Freq = 1 * timing.Hz
specA.PingDst = "AgentB.Out"
specA.NumPings = 2

specB := Definition.DefaultSpec
specB.Freq = 1 * timing.Hz

agentA := Definition.Builder().
	WithSimulation(sim).
	WithSpec(specA).
	Build("AgentA")
agentA.BindPort("Out", outA)

agentB := Definition.Builder().
	WithSimulation(sim).
	WithSpec(specB).
	Build("AgentB")
agentB.BindPort("Out", outB)

conn := direct.NewConnection("Conn", sim, timing.GHz)

conn.BindPort(agentA.Ports.Out)
conn.BindPort(agentB.Ports.Out)
if err := sim.Initialize(); err != nil {
	panic(err)
}

// AgentA sends pings on its own, so start it; AgentB wakes when a ping
// arrives.
agentA.TickLater()

err := engine.Run()
if err != nil {
	panic(err)
}
```

This code is the package's `Example` test, so it writes `Definition` and
`Ports`; a system builder outside the package writes
`tickingping.Definition` and `tickingping.Ports`.

The phases are explicit: configure the two Spec values, build the agents,
bind each `Out` port, and attach both ports to the connection. `Initialize`
creates State and Middlewares after wiring is complete. Then `TickLater`
seeds Agent A's first event and the engine runs.

`AgentB.Out` is a planned address in the configuration. An actual port's
`AsRemote()` is available only after its owner has bound it. Changing
`specA.NumPings` before `Build` changes that instance; changing the original
Spec after `Build` does not change the built component.

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
  `twowaybuffered.NewPort`, binds them with `component.BindPort`, and
  attaches them with `connection.BindPort`.
- **Ports buffer messages.** Messages are value types: construct them with
  no `&`, check `CanSend()` before `Send` because the outgoing buffer can
  be full, and `Peek` lets you look at incoming messages without consuming.
- **Connections move messages.** Plug ports into a `direct.Connection` and
  any plugged port can reach any other.
- **Components use builders; ports and connections use constructors.** Build
  components with `Definition.Builder().WithX().Build(name)`. Create ports with
  `twowaybuffered.NewPort(in, out)` and connections with
  `direct.NewConnection(name, sim, freq)`.

## Where to Next

You can build a working simulation with everything in these two sections.
The next section, **Getting Information from a Simulation**, shows how to
observe one: attach
callbacks that log events and messages, and measure how long work takes —
all without changing the components you just wrote.
