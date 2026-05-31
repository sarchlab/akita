package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// PingSpec is the immutable configuration for a ping component.
type PingSpec struct {
	OutPort messaging.Port
}

type ScheduledPing struct {
	SendAt timing.VTimeInSec
	Dst    messaging.RemotePort
}

// PendingResponse represents a ping response that will be sent after a delay.
type PendingResponse struct {
	DeliverAt timing.VTimeInSec
	Dst       messaging.RemotePort
	OrigMsgID uint64
	SeqID     int
}

// PingState is the mutable runtime state for a ping component.
type PingState struct {
	StartTimes       []timing.VTimeInSec
	NextSeqID        int
	PendingResponses []PendingResponse
	ScheduledPings   []ScheduledPing
}

// Comp is the ping component built on EventDrivenComponent.
type Comp = modeling.EventDrivenComponent[PingSpec, PingState, modeling.None]
