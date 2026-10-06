package tickingping

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// pingReq is a ping request message.
type pingReq struct {
	SeqID int
}

// pingRsp is a ping response message.
type pingRsp struct {
	SeqID int
}

// Spec contains immutable configuration for the tickingping component.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// PingDst is the port the component sends its pings to.
	PingDst messaging.RemotePort `json:"ping_dst"`

	// NumPings is the number of pings the component sends, one per cycle
	// from its first tick. Zero makes a component that only responds.
	NumPings int `json:"num_pings"`
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
	CurrentTransactions []pingTransactionState `json:"current_transactions"`
}

// Ports holds the tickingping component's ports.
type Ports struct {
	// Out sends pings and responses and receives them from the peer.
	Out messaging.Port
}

// Middlewares holds the tickingping component's behavior, run in this order
// on every tick.
type Middlewares struct {
	// Send sends a due response or the next ping.
	Send *sendMW

	// ReceiveProcess counts down the pings being answered and takes incoming
	// messages.
	ReceiveProcess *receiveProcessMW
}

// Comp is the tickingping component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

var _ = messaging.DefineProtocol(messaging.RoleDef{Name: "peer", Sends: []any{pingReq{}, pingRsp{}}})
