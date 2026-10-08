package messaging

import (
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/naming"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
// All four port message hook positions carry the Port as Domain and a Msg as Item.
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

	SetConnection(conn Connection)
	Connection() Connection
	Owner() PortOwner
	SetOwner(owner PortOwner)

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

	// Buffer counts
	NumIncoming() int
	NumOutgoing() int
}
