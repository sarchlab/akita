package timing

import "github.com/sarchlab/akita/v4/v5/instrumentation/hooking"

// Handler processes events of various types. Event payloads typically use type
// assertions within Handle implementations.
type Handler interface {
	Handle(event Event) error
}

// Hook positions emitted by timing engines.
var (
	HookPosBeforeEvent = &hooking.HookPos{Name: "TimingBeforeEvent"}
	HookPosAfterEvent  = &hooking.HookPos{Name: "TimingAfterEvent"}
)

// Event models a scheduled occurrence within the timing engines. Concrete event
// implementations provide their execution time and the handler that should
// receive the event value.
type Event interface {
	Time() VTimeInCycle
	Handler() Handler
}
