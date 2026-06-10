package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// pingReq is a ping request message.
type pingReq struct {
	messaging.MsgMeta
	SeqID int
}

// pingRsp is a ping response message.
type pingRsp struct {
	messaging.MsgMeta
	SeqID int
}

// pingProtocol is the ping protocol: ping agents are symmetric peers, each
// sending requests and answering with responses on the same port. Defining
// the protocol registers the message types with the checkpoint codec.
var (
	pingProtocol = messaging.DefineProtocol("examples.tickingping",
		messaging.RoleDef{Name: "peer",
			Sends: []messaging.Msg{pingReq{}, pingRsp{}}},
	)
	pingPeer = pingProtocol.Role("peer")
)

// Spec contains immutable configuration for the tickingping component.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// pingTransactionState tracks an in-progress ping request with a countdown.
type pingTransactionState struct {
	SeqID     int                  `json:"seq_id"`
	CycleLeft int                  `json:"cycle_left"`
	ReqID     uint64               `json:"req_id"`
	ReqSrc    messaging.RemotePort `json:"req_src"`
}

// State contains mutable runtime data for the tickingping component.
type State struct {
	StartTimes          []uint64               `json:"start_times"`
	NextSeqID           int                    `json:"next_seq_id"`
	NumPingNeedToSend   int                    `json:"num_ping_need_to_send"`
	PingDst             messaging.RemotePort   `json:"ping_dst"`
	CurrentTransactions []pingTransactionState `json:"current_transactions"`
}

// Comp is the tickingping component.
type Comp = modeling.Component[Spec, State, modeling.None]
