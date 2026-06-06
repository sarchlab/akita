# messaging — Messages, Ports, and Connections

Package `messaging` provides messages, ports, and connections for the Akita
simulation framework. It is the communication layer: components own ports,
ports buffer messages, and a connection moves messages from one port's outgoing
buffer to another port's incoming buffer.

## Key Concepts

- A **message** (`Msg`) is any value carrying a `*MsgMeta` with routing and
  identification metadata.
- A **port** is owned by a component and holds an incoming and an outgoing
  buffer. Components `Send`/`RetrieveIncoming` on their side; connections
  `Deliver`/`RetrieveOutgoing` on theirs.
- A **connection** is plugged into a set of ports and is responsible for
  delivering each outgoing message to the destination port named in its
  metadata.
- Ports are addressed by `RemotePort`, a string name. A message's `Src` and
  `Dst` are remote port names, not pointers.

## Key Types

### Msg and MsgMeta

```go
type Msg interface {
    Meta() *MsgMeta
}

type MsgMeta struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64 // ID of the request this responds to, if any
    SendTaskID   uint64
    RecvTaskID   uint64
}
```

Embed `MsgMeta` (or hold one) so a message satisfies `Msg`. `meta.IsRsp()`
reports whether `RspTo` is set.

### Port

```go
type Port interface {
    naming.Named
    hooking.Hookable

    AsRemote() RemotePort

    // For the component side
    CanSend() bool
    Send(msg Msg) *SendError
    PeekIncoming() Msg
    RetrieveIncoming() Msg

    // For the connection side
    Deliver(msg Msg) *SendError
    PeekOutgoing() Msg
    RetrieveOutgoing() Msg
    NotifyAvailable()

    SetConnection(conn Connection)
    Component() Component
    SetComponent(comp Component)
    NumIncoming() int
    NumOutgoing() int
}
```

Create a port with `NewPort`:

```go
port := messaging.NewPort(comp, incomingCap, outgoingCap, "MyComp.Top")
```

`Send` pushes onto the outgoing buffer (returning a `*SendError` if full) and,
when the buffer transitions from empty, notifies the connection via
`NotifySend`. `Deliver` pushes onto the incoming buffer and notifies the owning
component via `NotifyRecv`.

### Connection

```go
type Connection interface {
    naming.Named
    hooking.Hookable

    PlugIn(port Port)
    Unplug(port Port)
    NotifyAvailable(port Port)
    NotifySend()
}
```

A connection moves messages from outgoing to incoming buffers. `directconnection`
is the simplest implementation.

### Component and PortOwner

```go
type Component interface {
    naming.Named
    hooking.Hookable
    PortOwner

    NotifyRecv(port Port)
    NotifyPortFree(port Port)
}

type PortOwner interface {
    DeclarePort(name string)
    AssignPort(name string, port Port)
    AddPort(name string, port Port)
    GetPortByName(name string) Port
    Ports() []Port
}
```

Embed `PortOwnerBase` (via `NewPortOwnerBase`) to manage a named set of ports.
A component owns its port topology: it declares its ports with `DeclarePort`
(typically in its builder), and setup code supplies the instances with
`AssignPort`. `AddPort` is the legacy one-step path that declares and assigns
together, used by components that still create their own ports. `GetPortByName`
panics with a helpful message if the name is unknown or was declared but not
yet assigned.

## How It Works

1. A component declares its ports with `DeclarePort`; setup code builds each
   port with `NewPort` and attaches it with `AssignPort` (components that have
   not yet been migrated still create and add their ports in one step with
   `AddPort`).
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
