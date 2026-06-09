package packetization

import (
	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the link-level transport protocol: endpoints and switches
// exchange flits over network links. Flits are symmetric link traffic, so the
// protocol has a single role. Defining the protocol registers the flit type
// with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("packetization",
		messaging.RoleDef{Name: "link",
			Sends: []messaging.Msg{Flit{}}},
	)
	Link = Protocol.Role("link")
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	messaging.MsgMeta
	SeqID        int               `json:"seq_id"`
	NumFlitInMsg int               `json:"num_flit_in_msg"`
	Msg          messaging.MsgMeta `json:"msg"` // carried message metadata
}
