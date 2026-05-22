package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// PingReq is a ping request message.
type PingReq struct {
	messaging.MsgMeta
	SeqID int
}

// PingRsp is a ping response message.
type PingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// Spec contains immutable configuration for the tickingping component.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}
