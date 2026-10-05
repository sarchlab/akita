package ping

import (
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/wakeup"
	"github.com/sarchlab/akita/v5/simulation/timing"
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

// Ports holds the ping component's ports.
type Ports struct {
	// Out sends pings and responses and receives them from the peer.
	Out messaging.Port
}

// Middlewares holds the ping component's behavior.
type Middlewares struct {
	// Ping sends due pings and responses and handles incoming messages.
	Ping *pingMW
}

// Comp is a ping component, a wakeup component: it runs when a message
// arrives or when a ping or response it holds becomes due.
type Comp = wakeup.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the ping component.
var Definition = wakeup.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Ping: &pingMW{comp: c}}
}

// SchedulePing schedules a ping to be sent at the given time to the given
// destination.
func SchedulePing(
	comp *Comp,
	sendAt timing.VTimeInPicoSec,
	dst messaging.RemotePort,
) {
	state := &comp.State
	state.ScheduledPings = append(state.ScheduledPings, scheduledPing{
		SendAt: sendAt,
		Dst:    dst,
	})
	comp.WakeAt(sendAt)
}
