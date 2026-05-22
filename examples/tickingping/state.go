package tickingping

import (
	"github.com/sarchlab/akita/v5/messaging"
)

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
