# direct — Ideal Direct Connection

Package `direct` provides a simple network connection for the Akita
simulation framework. It connects any number of ports and delivers messages
between them in a single tick, with no routing overhead or bandwidth modeling.

## How It Works

A `Connection` maintains a set of plugged-in ports. Each tick, it round-robins
through all ports, peeks at each port's outgoing buffer, resolves the
destination port by name, and delivers the message directly. Messages are
delivered in the same tick they are sent (subject to the connection's tick
frequency).

This makes `direct.Connection` ideal for connecting components that are
logically adjacent — for example, a cache and its memory controller, or
a compute unit and its L1 cache — without modeling a full network-on-chip.

## Key Types

### Connection

```go
type Connection struct {
    State State // saved in checkpoints
    // ...
}

func (c *Connection) BindPort(port messaging.Port)          // Connect a port
func (c *Connection) NotifyAvailable(p messaging.Port)    // Port buffer space freed
func (c *Connection) NotifySend()                         // Port has outgoing message
```

`Connection` implements `messaging.Connection`, so ports can use it as their
connection for message delivery. It is a connection, not a component: ports plug
into it during wiring. It ticks on secondary tick events, so it runs after the
components of the same cycle. Its constructor takes an explicit tick frequency.

## Construction

Pass the name, simulation, and frequency to `NewConnection`. The constructor
uses the simulation's engine and registers the connection and its event handler.
It retains the frequency internally for checkpoint validation; no public
`Definition`, `Spec`, or builder is needed.

```go
conn := direct.NewConnection("Connection", s, timing.GHz)
conn.BindPort(portA)
conn.BindPort(portB)
conn.BindPort(portC)
```

## Usage

```go
// Create engine and connection
engine := timing.NewSerialEngine()
sim := modeling.NewStandaloneSimulation(engine)
conn := direct.NewConnection("Bus", sim, timing.GHz)

// Create components with ports, then plug them in
conn.BindPort(cache.Ports.Bottom)
conn.BindPort(memCtrl.Ports.Top)
```

When component A sends a message to component B:
1. A places the message in its port's outgoing buffer with `Dst` set to B's
   port name.
2. The connection ticks, finds the message, resolves `Dst` to B's port, and
   calls `Deliver()`.
3. B receives the message in its port's incoming buffer on the next tick.

## Fairness

The connection uses round-robin scheduling across ports. The starting port
index advances by one each tick, ensuring no port is permanently starved.

## Limitations

- No bandwidth modeling — all pending messages are forwarded each tick.
- No latency modeling beyond the tick granularity.
- `noc/networking` adds switch pipelines, routing, and contention while using
  ideal direct links. Finite link bandwidth and latency are separate future work.
