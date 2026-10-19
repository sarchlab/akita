package ticking

import (
	"github.com/sarchlab/akita/v5/internal/tickscheduler"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Event is the event a ticking component receives while it makes progress.
type Event = tickscheduler.Event

// MakeEvent creates a tick event for the given handler and time.
func MakeEvent(id uint64, time timing.VTimeInPicoSec, handlerID string) Event {
	return tickscheduler.MakeEvent(id, time, handlerID)
}
