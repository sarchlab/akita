# messaging — Messages, Ports, and Connections

Package `messaging` provides messages, ports, and connections for the Akita
simulation framework. It is the communication layer: components own ports,
ports buffer messages, and a connection moves messages from one port's outgoing
buffer to another port's incoming buffer.

## Implementations

The parent package owns the common interfaces, message values, and protocols.
Concrete transports live in separate packages:

- [`twowaybuffered`](./twowaybuffered/README.md) provides `Port` with independent
  incoming and outgoing buffers. It works with direct connections and NoC endpoints.
- [`direct`](./direct/README.md) provides the ideal `Connection`, created with
  `NewConnection(name, simulation, frequency)`.
- `sim/messaging/wire` is reserved for the future wire port and connection; wire
  support is not implemented in this change.

Components keep `messaging.Port` fields. System builders select the concrete
port and connection implementations. NoC builders support only
`twowaybuffered.Port` and `direct.Connection`.

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
payloads, including pointers, and accepts a nil payload. Declare protocols as
package-level variables so `inspect` can discover them and every fresh process
registers the payload types before restoring a checkpoint. This is a convention,
not an init-only runtime check. The registry supports concurrent registration;
`Send` reads an immutable snapshot with an atomic load and a map lookup.

Custom ports can call `messaging.PayloadRegistered(payload)` to perform the same
registration check as `twowaybuffered.Port.Send`, including from another module.
It returns true for nil or a registered value type, and false for unregistered
types, including pointers. This read-only check does not register or serialize
the payload; the registry remains private.

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

    BindConnection(conn Connection)
    Owner() PortOwner
    BindOwner(owner PortOwner, name string)
    NumIncoming() int
    NumOutgoing() int
}
```

Create a buffered port with `twowaybuffered.NewPort`:

```go
port := twowaybuffered.NewPort(incomingCap, outgoingCap)
```

Call `component.BindPort("Top", port)` after building the component. This assigns
the full name and owner, fills the declared slot, and registers the port.

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

    BindPort(port Port)
    NotifyAvailable(port Port)
    NotifySend()
}
```

A connection moves messages from outgoing to incoming buffers. `direct.Connection`
in `sim/messaging/direct` is the simplest implementation.

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
each unnamed port and calls `component.BindPort("Field", port)`. Custom owners
call `port.BindOwner(owner, fullName)` and register their ports themselves.
A port must have an owner before `AsRemote` or message traffic.

## How It Works

1. Configure Spec structs and build components.
2. Bind component ports with `component.BindPort("Field", port)`.
3. Attach ports with `connection.BindPort(port)`, which calls the guarded
   reverse setter `port.BindConnection(connection)`.
4. Call `simulation.Initialize()` once, then seed work and run the engine.
5. Components send messages; connections drain outgoing buffers and deliver
   into destination incoming buffers, waking the receiving component.

## Hooks

Ports are `hooking.Hookable` and fire hooks (with the message as `HookCtx.Item`)
at `HookPosPortMsgSend`, `HookPosPortMsgRecvd`, `HookPosPortMsgRetrieveIncoming`,
and `HookPosPortMsgRetrieveOutgoing`, which tracing and instrumentation use to
observe traffic.
