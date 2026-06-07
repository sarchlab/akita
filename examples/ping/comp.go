package ping

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec is the immutable configuration for a ping component.
type Spec struct {
}

type scheduledPing struct {
	SendAt timing.VTimeInPicoSec
	Dst    messaging.RemotePort
}

// pendingResponse represents a ping response that will be sent after a delay.
type pendingResponse struct {
	DeliverAt timing.VTimeInPicoSec
	Dst       messaging.RemotePort
	OrigMsgID uint64
	SeqID     int
}

// State is the mutable runtime state for a ping component.
type State struct {
	StartTimes       []timing.VTimeInPicoSec
	NextSeqID        int
	PendingResponses []pendingResponse
	ScheduledPings   []scheduledPing
}

// Comp is the ping component built on EventDrivenComponent.
type Comp = modeling.EventDrivenComponent[Spec, State, modeling.None]
