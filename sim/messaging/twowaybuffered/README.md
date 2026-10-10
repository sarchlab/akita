# twowaybuffered — Buffered Ports

Package `sim/messaging/twowaybuffered` implements `messaging.Port` with separate
incoming and outgoing message buffers. It is shared by ideal direct connections
and NoC endpoints; it does not depend on either implementation.

```go
import "github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"

port := twowaybuffered.NewPort(8, 4)
```

`NewPort(incomingCapacity, outgoingCapacity)` returns `*twowaybuffered.Port`.
Create ports without names, then call `component.BindPort("Top", port)`.
For a component named `Cache`, this assigns `Cache.Top`, records the owner,
and registers the port. Custom owners call `port.BindOwner(owner, "Cache.Top")`
and register the port themselves. Binding is one-time; an unowned port has no
remote address. Attach it with `connection.BindPort(port)` during setup.
After all wiring, call `simulation.Initialize()` before sending work or running.

A component calls `CanSend` before `Send`, and reads `PeekIncoming` or
`RetrieveIncoming`. A connection reads `PeekOutgoing` or `RetrieveOutgoing` and
calls `CanDeliver` before `Deliver`. Reads return `(messaging.Msg{}, false)` when
empty; sending or delivering into a full buffer panics. Notifications wake the
connection or component when work or buffer space becomes available.

The port emits the hooks declared in `messaging`. `Send` checks the same shared
payload registry used by `messaging.Msg` serialization, rejecting unregistered
payload types before buffering them. Nil payloads remain valid.

`SaveCheckpoint` and `LoadCheckpoint` preserve both buffers, FIFO order, and
concrete payload types. Loading checks the rebuilt capacities. Hooks, owners,
and connections are supplied by setup rather than serialized. The port's
checkpoint format is unchanged by the move.

A port can attach to a compatible `messaging.Connection`, including
`direct.Connection` or a NoC endpoint's device-side connection. The NoC rejects
other device-port implementations at construction time. Wire ports have a
different transport contract and are not supported by NoC builders.
