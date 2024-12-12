package messaging

import (
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/timing"
)

// A TransferEvent is an event that marks that a message completes transfer.
type TransferEvent struct {
	*timing.EventBase
	msg modeling.Msg
	vc  int
}

// NewTransferEvent creates a new TransferEvent.
func NewTransferEvent(
	time timing.VTimeInSec,
	handler timing.Handler,
	msg modeling.Msg,
	vc int,
) *TransferEvent {
	evt := new(TransferEvent)
	evt.EventBase = timing.NewEventBase(time, handler)
	evt.msg = msg
	evt.vc = vc

	return evt
}
