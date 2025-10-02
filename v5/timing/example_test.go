package timing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v4/v5/timing"
)

// exampleScheduler captures the scheduling API the example component depends on.
type exampleScheduler interface {
	Schedule(timing.ScheduledEvent)
	CurrentTime() timing.VTimeInCycle
}

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
	engine exampleScheduler
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

// Example_serialEngine_basic shows how to schedule events without any clock-domain logic.
func Example_serialEngine_basic() {
	engine := timing.NewSerialEngine()
	component := &ExampleComponent{name: "mailbox", engine: engine}

	engine.Schedule(timing.ScheduledEvent{
		Event:   &PingEvent{Message: "hello", From: "client"},
		Time:    timing.VTimeInCycle(5),
		Handler: component,
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

// Example_frequencyRegistry_twoDomains illustrates how separate domains align to the global cycle.
func Example_frequencyRegistry_twoDomains() {
	registry := timing.NewFrequencyRegistry()
	cpuDomain, _ := registry.RegisterFrequency(2 * timing.GHz)
	memDomain, _ := registry.RegisterFrequency(1 * timing.GHz)

	fmt.Printf("cpu stride: %d\n", cpuDomain.Stride())
	fmt.Printf("memory stride: %d\n", memDomain.Stride())
	fmt.Printf("memory this tick at 3 cycles: %d\n", memDomain.ThisTick(timing.VTimeInCycle(3)))
	fmt.Printf("memory next tick after 4 cycles: %d\n", memDomain.NextTick(timing.VTimeInCycle(4)))
	fmt.Printf("memory two ticks later from 3 cycles: %d\n",
		memDomain.NTicksLater(timing.VTimeInCycle(3), timing.VTimeInCycle(2)))
	// Output:
	// cpu stride: 1
	// memory stride: 2
	// memory this tick at 3 cycles: 4
	// memory next tick after 4 cycles: 6
	// memory two ticks later from 3 cycles: 8
}
