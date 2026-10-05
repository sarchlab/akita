# messaging — Messages, Ports, and Connections

Package `messaging` provides messages, ports, and connections for the Akita
simulation framework. It is the communication layer: components own ports,
ports buffer messages, and a connection moves messages from one port's outgoing
buffer to another port's incoming buffer.

## Key Concepts

- A **message** (`Msg`) is a value with routing fields and a protocol-specific
  `Payload`. A nil payload is metadata-only traffic.
- A **protocol** (`Protocol`) registers value payload types during package
  initialization and groups them into **roles** (`Role`). Its name is the defining
  package's import path. Ports declare roles with tags such as
  `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`.
  Non-nil payloads must be registered before they can be sent. `AnyRole` accepts
  traffic from any protocol; it does not bypass payload registration.
- A **port** is owned by a component and holds an incoming and an outgoing
  buffer. Components `Send`/`RetrieveIncoming` on their side; connections
  `Deliver`/`RetrieveOutgoing` on theirs.
- A **connection** is plugged into a set of ports and is responsible for
  delivering each outgoing message to the destination port named in its
  metadata.
- Ports are addressed by `RemotePort`, a string name. A message's `Src` and
  `Dst` are remote port names, not pointers.

## Key Types

### Msg and payloads

```go
type Msg struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64
    Payload      any
}
```

`msg.IsRsp()` reports whether `RspTo` is nonzero. Routing fields live directly on
`Msg`; protocol types contain only their own fields:

```go
type ReadReq struct { Address uint64 }
type WriteDoneRsp struct{}

var Protocol = messaging.DefineProtocol(
    messaging.RoleDef{Name: "requester", Sends: []any{ReadReq{}}},
    messaging.RoleDef{Name: "responder", Sends: []any{WriteDoneRsp{}}},
)

port.Send(messaging.Msg{
    ID: id, Src: port.AsRemote(), Dst: lower,
    Payload: ReadReq{Address: 64},
})
```

Registration rejects pointer prototypes and pointer, interface, channel,
function, and unsafe-pointer fields at any depth, even with `json:"-"` or custom
JSON methods. Scalars, structs, arrays, slices, maps with supported JSON keys,
and nested `messaging.Msg` values are allowed. `Send` rejects unregistered
payloads, including pointers, and accepts a nil payload. Protocol definitions
must run during package initialization; runtime registry lookups do not lock.

Slice and map storage is shared between sender and receiver. Do not mutate it
after sending. Inspect a payload with `switch req := msg.Payload.(type)` while
using `msg.ID`, `msg.Src`, and the other envelope fields for routing and replies.

`Msg` implements JSON serialization itself, preserving the payload's concrete
type with a full-import-path tag. It can be stored directly in component state,
port buffers, or another payload. A nil payload omits both `type` and `payload`.
The checkpoint shape is a readable object with routing fields, `type`, and
`payload`.

Declare roles in the protocol and bind ports to roles with a tag on the component's Ports field:

```go
Top messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.responder"`
```

The inspector checks each tag against the protocol's roles. Each protocol lives in its
own package (e.g. `mem/memprotocol`, `mem/memcontrolprotocol`, `mem/vm/vmprotocol`)
that owns the message types and the protocol definition. A
registration-coverage audit
(`protocolaudit_test.go`) fails CI for concrete payloads constructed in library message literals that are
not registered. Events are registered with `timing.RegisterEvent`. No custom
marshalling is needed.
See [`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md).

### Port

```go
type Port interface {
    naming.Named
    hooking.Hookable

    AsRemote() RemotePort

    // For the component side
    CanSend() bool
    Send(msg Msg)
    PeekIncoming() (Msg, bool)
    RetrieveIncoming() (Msg, bool)

    // For the connection side
    Deliver(msg Msg)
    PeekOutgoing() (Msg, bool)
    RetrieveOutgoing() (Msg, bool)
    NotifyAvailable()

    SetConnection(conn Connection)
    Owner() PortOwner
    SetOwner(owner PortOwner)
    NumIncoming() int
    NumOutgoing() int
}
```

Create a port with `NewPort`:

```go
port := messaging.NewPort("MyComp.Top", incomingCap, outgoingCap)
```

In assembly, the system builder creates each port with no component and passes
it to the component's `Build`, which binds the port to the component and
registers it with the simulation (and the monitor).

`Send` pushes onto the outgoing buffer — callers must check `CanSend` first;
sending into a full buffer panics — and, when the buffer transitions from
empty, notifies the connection via `NotifySend`. `Deliver` pushes onto the
incoming buffer (guarded by `CanDeliver` the same way) and notifies the owning
component via `NotifyRecv`.

Reads return `(Msg{}, false)` when empty. `Peek` leaves the message queued;
`Retrieve` consumes it and retains the sender notification when a full buffer
gains space. A successful read returns `true`. Capacity and peek checks do not
reserve buffer space or messages.

### Connection

```go
type Connection interface {
    naming.Named
    hooking.Hookable

    PlugIn(port Port)
    NotifyAvailable(port Port)
    NotifySend()
}
```

A connection moves messages from outgoing to incoming buffers. `directconnection`
is the simplest implementation.

### PortOwner

```go
type PortOwner interface {
    NotifyRecv(port Port)
    NotifyPortFree(port Port)
}
```

A port notifies its owner when a message arrives in an empty incoming buffer
and when it can send again. The owner is usually a component
(`modeling.Component`), but `messaging` asks only for these two methods. The
interface says nothing about which ports an owner has or how it reaches them.
A component defined by a component model (`modeling/ticking` and its
siblings) holds its ports in a typed `Ports` struct: the system builder creates
each port with `NewPort` and passes them all to `Build`, which binds and
registers them. An owner written without a component model, such as a test
driver, calls `SetOwner` itself. A port must have an owner before it carries
traffic: `Deliver`, `RetrieveOutgoing`, and `NotifyAvailable` panic on a port
without one.

## How It Works

1. Setup code creates each port with `NewPort` and passes it to its component's
   `Build`.
2. A connection is plugged into the ports with `PlugIn`, and each port's
   connection is set with `SetConnection`.
3. To send, a component builds a message with `Src`/`Dst` remote port names and
   calls `port.Send(msg)`.
4. The connection picks up the message via `PeekOutgoing`/`RetrieveOutgoing`,
   resolves `Dst`, and calls the destination port's `Deliver`.
5. The destination component is notified by `NotifyRecv` and consumes the
   message with `PeekIncoming`/`RetrieveIncoming`.

## Hooks

Ports are `hooking.Hookable` and fire hooks (with the message as `HookCtx.Item`)
at `HookPosPortMsgSend`, `HookPosPortMsgRecvd`, `HookPosPortMsgRetrieveIncoming`,
and `HookPosPortMsgRetrieveOutgoing`, which tracing and instrumentation use to
observe traffic.
