package tickingping

import (
	"github.com/sarchlab/akita/v5/sim"
)

// PingReq is a ping request message.
type PingReq struct {
	sim.MsgMeta
	SeqID int
}

// PingRsp is a ping response message.
type PingRsp struct {
	sim.MsgMeta
	SeqID int
}

// Spec contains immutable configuration for the tickingping component.
type Spec struct {
	Freq sim.Freq `json:"freq"`
}
