package messaging

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/naming"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
// All four port message hook positions carry the Port as Domain, a Msg as Item,
// and PortBufferTrace as Detail. Hooks must not call buffer operations (peek, count, or mutate): the port
// may hold its buffer lock while notifying them.
var HookPosPortMsgSend = &hooking.HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at the port.
var HookPosPortMsgRecvd = &hooking.HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieveIncoming marks when an inbound message is retrieved
// from the incoming buffer.
var HookPosPortMsgRetrieveIncoming = &hooking.HookPos{
	Name: "Port Msg Retrieve Incoming",
}

// HookPosPortMsgRetrieveOutgoing marks when an outbound message is retrieved
// from the outgoing buffer.
var HookPosPortMsgRetrieveOutgoing = &hooking.HookPos{
	Name: "Port Msg Retrieve Outgoing",
}

// PortBufferTrace describes the buffer task affected by a port hook. TaskID is
// the enqueued or removed entry's task. HeadTaskID is the task that just reached
// the head (zero if none). These IDs are stored in the port checkpoint.
type PortBufferTrace struct {
	TaskID     uint64
	HeadTaskID uint64
}

// A RemotePort is a string that refers to another port.
type RemotePort string

// A Port is owned by a component and is used to plug in connections.
// Peek and Retrieve return Msg{}, false when the corresponding buffer is empty.
// Retrieve removes a message and notifies its sender when a full buffer gains
// space. Capacity and peek checks do not reserve space or messages.
type Port interface {
	naming.Named
	hooking.Hookable

	AsRemote() RemotePort

	BindConnection(conn Connection)
	Connection() Connection
	Owner() PortOwner
	BindOwner(owner PortOwner, name string)

	// For connection
	CanDeliver() bool
	Deliver(msg Msg)
	NotifyAvailable()
	RetrieveOutgoing() (Msg, bool)
	PeekOutgoing() (Msg, bool)

	// For component
	CanSend() bool
	Send(msg Msg)
	RetrieveIncoming() (Msg, bool)
	PeekIncoming() (Msg, bool)

	// PeekIncomingTaskID returns the head entry's buffer task ID, or zero
	// when empty or untraced. Read it before RetrieveIncoming.
	PeekIncomingTaskID() uint64

	// Buffer counts
	NumIncoming() int
	NumOutgoing() int
}
