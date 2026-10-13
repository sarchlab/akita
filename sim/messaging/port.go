package messaging

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/naming"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
// All four port message hook positions carry the Port as Domain, a Msg as Item,
// and nil Detail. Hooks must not call buffer operations (peek, count, or mutate): the port
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

// WaitReason describes why a message waited at the head of a port buffer.
// An empty Kind selects the port's default reason (consumer for incoming,
// network_busy for outgoing); What is then supplied by the port as well.
type WaitReason struct {
	Kind string
	What string
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

	// AddIncomingMilestone records the completed wait since the incoming head's
	// previous milestone, or since it became the head. It does not remove the
	// message. Empty buffers, untracked messages, and zero-length waits are no-ops.
	// Call at a wait-stage transition, not on each unsuccessful polling attempt.
	AddIncomingMilestone(reason WaitReason)

	// AddOutgoingMilestone is the corresponding operation for the outgoing head.
	// Retrieve adds the default reason only for time left since the last milestone.
	AddOutgoingMilestone(reason WaitReason)

	// Buffer counts
	NumIncoming() int
	NumOutgoing() int
}
