package timing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v4/v5/timing"
)

// exampleScheduler captures the scheduling API the example component depends on.
type exampleScheduler interface {
	Schedule(timing.Event)
	CurrentTime() timing.VTimeInCycle
}

// PingEvent represents a ping request.
type PingEvent struct {
	Cycle   timing.VTimeInCycle
	Target  timing.Handler
	Message string
	From    string
}

// Time returns when the event should trigger.
func (e *PingEvent) Time() timing.VTimeInCycle { return e.Cycle }

// Handler returns who should handle the event.
func (e *PingEvent) Handler() timing.Handler { return e.Target }

// PongEvent represents a pong response.
type PongEvent struct {
	Cycle  timing.VTimeInCycle
	Target timing.Handler
	Reply  string
	To     string
}

// Time returns when the event should trigger.
func (e *PongEvent) Time() timing.VTimeInCycle { return e.Cycle }

// Handler returns who should handle the event.
func (e *PongEvent) Handler() timing.Handler { return e.Target }

// ExampleComponent demonstrates how to create a component that handles events.
type ExampleComponent struct {
	name   string
	engine exampleScheduler
}

// Handle processes events using type switching - the Go-idiomatic way.
func (c *ExampleComponent) Handle(event any) error {
	switch e := event.(type) {
	case *PingEvent:
		fmt.Printf("[%s] Received ping from %s: %s\n", c.name, e.From, e.Message)
		// Schedule a response
		c.engine.Schedule(&PongEvent{
			Cycle:  c.engine.CurrentTime() + timing.VTimeInCycle(1),
			Target: c,
			Reply:  "pong!",
			To:     e.From,
		})
	case *PongEvent:
		fmt.Printf("[%s] Received pong to %s: %s\n", c.name, e.To, e.Reply)
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
	return nil
}

// Example_serialEngine_basic shows how to schedule events without any clock-domain logic.
func Example_serialEngine_basic() {
	engine := timing.NewSerialEngine()
	component := &ExampleComponent{name: "mailbox", engine: engine}

	engine.Schedule(&PingEvent{
		Cycle:   timing.VTimeInCycle(5),
		Target:  component,
		Message: "hello",
		From:    "client",
	})

	if err := engine.Run(); err != nil {
		fmt.Println("unexpected error:", err)
		return
	}

	fmt.Printf("current time: %d cycles\n", engine.CurrentTime())
	// Output:
	// [mailbox] Received ping from client: hello
	// [mailbox] Received pong to client: pong!
	// current time: 6 cycles
}
