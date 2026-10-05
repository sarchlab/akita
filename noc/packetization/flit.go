package packetization

import "github.com/sarchlab/akita/v5/simulation/messaging"

// Protocol is the traffic-only transport protocol. On the link role,
// endpoints and switches exchange flits over network links (symmetric link
// traffic). On the delivery role, an endpoint delivers the reassembled
// message to the destination device port. Defining the protocol registers
// both message types with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "link",
			Sends: []any{Flit{}}},
		messaging.RoleDef{Name: "delivery",
			Sends: []any{AssembledMsg{}}},
	)
	Link     = Protocol.Role("link")
	Delivery = Protocol.Role("delivery")
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	SeqID        int           `json:"seq_id"`
	NumFlitInMsg int           `json:"num_flit_in_msg"`
	Msg          messaging.Msg `json:"msg"` // carried message metadata
	// MsgTaskID is the tracing task ID of the carried message's end-to-end
	// (msg_e2e) task. The sending endpoint generates it once per message (a
	// unique ID, distinct from the message's own ID), stamps it on every flit,
	// and parents each flit_e2e task to it; the receiving endpoint reads it back
	// to close the msg_e2e task.
	MsgTaskID uint64 `json:"msg_task_id"`
}

// AssembledMsg identifies traffic-only network delivery. The receiving endpoint
// attaches it to the carried routing fields after reassembly. Application
// payload delivery is tracked separately in #495.
type AssembledMsg struct {
}
