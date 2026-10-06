package event

import "github.com/sarchlab/akita/v5/sim/timing"

// Recv is the event an event component receives when one of its ports
// receives a message into an empty incoming buffer. Port is the port's full
// name; compare it with the Name of a Ports field. The port is not notified
// again until its incoming buffer empties, so a middleware that handles Recv
// takes every message it can and schedules a retry for the rest.
type Recv struct {
	timing.EventBase
	Port string `json:"port"`
}

// PortFree is the event an event component receives when one of its ports
// can send again: its full outgoing buffer gained a slot, or the connection
// signaled that it can take messages again. Port is the port's full name.
type PortFree struct {
	timing.EventBase
	Port string `json:"port"`
}

// init registers the event types so a checkpoint can hold them while they are
// pending.
func init() {
	timing.RegisterEvent(Recv{})
	timing.RegisterEvent(PortFree{})
}
