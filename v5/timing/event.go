package timing

// Handler processes events of various types.
// Events are plain data structs (no interface required).
// Handlers use type switching to handle different event types:
//
//	func (h *MyHandler) Handle(event any) error {
//	    switch e := event.(type) {
//	    case *MyEvent:
//	        // handle MyEvent
//	    case *AnotherEvent:
//	        // handle AnotherEvent
//	    default:
//	        return fmt.Errorf("unknown event type: %T", event)
//	    }
//	    return nil
//	}
type Handler interface {
	Handle(event any) error
}

// TimeTeller exposes the current simulation cycle.
type TimeTeller interface {
	CurrentTime() VTimeInCycle
}

// EventScheduler schedules events in the simulation timeline.
type EventScheduler interface {
	TimeTeller
	Schedule(event ScheduledEvent)
}

// ScheduledEvent is the engine-facing wrapper for user-defined events.
// It holds the metadata needed by the scheduler while keeping the payload as
// plain data. Users typically pass pointers so large structs are not copied.
type ScheduledEvent struct {
	// Event is the data payload to be delivered to the handler.
	// Can be any type - typically a pointer to a struct defined by the user.
	Event any

	// Time is the cycle when the event should be processed.
	Time VTimeInCycle

	// Handler is the component that will process this event.
	Handler Handler

	// IsSecondary indicates if this event should be processed after
	// all primary events at the same time. Secondary events are useful
	// for cleanup or state synchronization tasks.
	IsSecondary bool
}
