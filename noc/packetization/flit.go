package packetization

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the traffic-only transport protocol. On the link role,
// endpoints and switches exchange flits over network links (symmetric link
// traffic). On the delivery role, an endpoint delivers the reassembled
// message to the destination device port. Defining the protocol registers
// both message types with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("packetization",
		messaging.RoleDef{Name: "link",
			Sends: []messaging.Msg{Flit{}}},
		messaging.RoleDef{Name: "delivery",
			Sends: []messaging.Msg{AssembledMsg{}}},
	)
	Link     = Protocol.Role("link")
	Delivery = Protocol.Role("delivery")
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	messaging.MsgMeta
	SeqID        int               `json:"seq_id"`
	NumFlitInMsg int               `json:"num_flit_in_msg"`
	Msg          messaging.MsgMeta `json:"msg"` // carried message metadata
	// MsgTaskID is the tracing task ID of the carried message's end-to-end
	// (msg_e2e) task. The sending endpoint generates it once per message (a
	// unique ID, distinct from the message's own ID), stamps it on every flit,
	// and parents each flit_e2e task to it; the receiving endpoint reads it back
	// to close the msg_e2e task.
	MsgTaskID uint64 `json:"msg_task_id"`
}

// AssembledMsg is what an endpoint delivers to a device port in place of the
// original message. The network is a traffic-only model: the endpoint strips
// an outgoing message down to its metadata, carries the metadata in flits,
// and reassembles it at the far end. Receivers under this model only ever see
// the metadata. Bare MsgMeta is the envelope and belongs to no protocol, so
// the reassembled metadata is delivered in this concrete wrapper.
type AssembledMsg struct {
	messaging.MsgMeta
}
