package modeling

import "github.com/sarchlab/akita/v5/timing"

// init registers the built-in modeling event types so the engine's event queue
// can be checkpointed when these events are pending. TickEvent and
// WakeupEvent are scheduled by value; TimerFiredEvent is scheduled as a
// pointer.
func init() {
	timing.RegisterEvent(TickEvent{})
	timing.RegisterEvent(WakeupEvent{})
	timing.RegisterEvent(TimerFiredEvent{})
}
