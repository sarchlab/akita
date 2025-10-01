package timing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v4/v5/timing"
)

// Example custom event types - pure data structs with no behavior.

// PingEvent represents a ping request.
type PingEvent struct {
	Message string
	From    string
}

// PongEvent represents a pong response.
type PongEvent struct {
	Reply string
	To    string
}

// ExampleComponent demonstrates how to create a component that handles events.
type ExampleComponent struct {
	name   string
	engine timing.EventScheduler
}

// Handle processes events using type switching - the Go-idiomatic way.
func (c *ExampleComponent) Handle(event any) error {
	switch e := event.(type) {
	case *PingEvent:
		fmt.Printf("[%s] Received ping from %s: %s\n", c.name, e.From, e.Message)
		// Schedule a response
		c.engine.Schedule(timing.ScheduledEvent{
			Event: &PongEvent{
				Reply: "pong!",
				To:    e.From,
			},
			Time:    c.engine.CurrentTime() + timing.VTimeInCycle(1),
			Handler: c,
		})
	case *PongEvent:
		fmt.Printf("[%s] Received pong to %s: %s\n", c.name, e.To, e.Reply)
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
	return nil
}

// ExampleEvent demonstrates the basic usage pattern for events and handlers.
func Example_eventUsage() {
	// This example shows how users define events as pure data structs
	// and use type switching in handlers.

	// 1. Define events as plain structs (shown above: PingEvent, PongEvent)

	// 2. Create a handler that processes events via type switching
	//    (shown above: ExampleComponent.Handle)

	// 3. Schedule events using ScheduledEvent
	//    engine.Schedule(timing.ScheduledEvent{
	//        Event:       &PingEvent{Message: "hello", From: "Alice"},
	//        Time:        timing.VTimeInCycle(10),
	//        Handler:     myHandler,
	//        IsSecondary: false,
	//    })

	fmt.Println("Events are pure data structs")
	fmt.Println("Handlers use type switching to process different event types")
	fmt.Println("Engine wraps events internally with timing/routing metadata")
	// Output:
	// Events are pure data structs
	// Handlers use type switching to process different event types
	// Engine wraps events internally with timing/routing metadata
}
