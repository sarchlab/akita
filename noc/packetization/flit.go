package packetization

import "github.com/sarchlab/akita/v5/sim/messaging"

// Protocol is the network transport protocol. Endpoints and switches exchange
// flits through the symmetric link role. Device ports receive the original
// application message after reassembly, not a transport-specific payload.
var (
	Protocol = messaging.DefineProtocol(
		messaging.RoleDef{Name: "link", Sends: []any{Flit{}}},
	)
	Link = Protocol.Role("link")
)

// Flit represents one network transfer unit. Every flit carries the header
// needed for independent routing and reassembly; only flit 0 carries Msg.
type Flit struct {
	MsgID        uint64               `json:"msg_id"`
	Dst          messaging.RemotePort `json:"dst"`
	SeqID        int                  `json:"seq_id"`
	NumFlitInMsg int                  `json:"num_flit_in_msg"`

	// Msg is the complete original message on flit 0 and the zero value on
	// all other flits. Omitting zero messages avoids repeating message metadata
	// in checkpoints. SeqID identifies the head even when Msg.Payload is nil.
	Msg messaging.Msg `json:"msg,omitzero"`

	// MsgTaskID identifies the message's end-to-end tracing task. It is distinct
	// from MsgID and travels on every flit so arrival order does not affect tracing.
	MsgTaskID uint64 `json:"msg_task_id"`
}
