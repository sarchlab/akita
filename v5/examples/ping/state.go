package ping

import "github.com/sarchlab/akita/v5/sim"

// ScheduledPing represents a ping that should be initiated at a given time.
type ScheduledPing struct {
	SendAt sim.VTimeInSec
	Dst    sim.RemotePort
}

// PendingResponse represents a ping response that will be sent after a delay.
type PendingResponse struct {
	DeliverAt sim.VTimeInSec
	Dst       sim.RemotePort
	OrigMsgID string
	SeqID     int
}

// PingState is the mutable runtime state for a ping component.
type PingState struct {
	StartTimes       []sim.VTimeInSec
	NextSeqID        int
	PendingResponses []PendingResponse
	ScheduledPings   []ScheduledPing
}
