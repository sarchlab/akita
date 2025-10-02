package timing_test

import (
	"fmt"

	"github.com/sarchlab/akita/v4/v5/timing"
)

type frequencyAwareComponent struct {
	name   string
	engine exampleScheduler
	domain *timing.FreqDomain
}

func (c *frequencyAwareComponent) Handle(event any) error {
	switch e := event.(type) {
	case *PingEvent:
		fmt.Printf("[%s] tick %d ping from %s: %s\n", c.name, c.engine.CurrentTime(), e.From, e.Message)
		nextTick := c.domain.NextTick(c.engine.CurrentTime())
		c.engine.Schedule(timing.ScheduledEvent{
			Event:   &PongEvent{Reply: "pong!", To: e.From},
			Time:    nextTick,
			Handler: c,
		})
	case *PongEvent:
		fmt.Printf("[%s] tick %d pong to %s: %s\n", c.name, c.engine.CurrentTime(), e.To, e.Reply)
	default:
		return fmt.Errorf("unknown event type: %T", event)
	}
	return nil
}

// Example_frequencyRegistry_twoDomains illustrates how separate domains align to the global cycle.
func Example_frequencyRegistry_twoDomains() {
	registry := timing.NewFrequencyRegistry()
	cpuDomain, _ := registry.RegisterFrequency(2 * timing.GHz)
	memDomain, _ := registry.RegisterFrequency(1 * timing.GHz)

	engine := timing.NewSerialEngine()
	component := &frequencyAwareComponent{name: "mailbox", engine: engine, domain: memDomain}

	engine.Schedule(timing.ScheduledEvent{
		Event:   &PingEvent{Message: "hello", From: "client"},
		Time:    memDomain.ThisTick(0),
		Handler: component,
	})

	if err := engine.Run(); err != nil {
		fmt.Println("unexpected error:", err)
		return
	}

	fmt.Printf("cpu stride: %d\n", cpuDomain.Stride())
	fmt.Printf("memory stride: %d\n", memDomain.Stride())
	fmt.Printf("final time: %d cycles\n", engine.CurrentTime())
	// Output:
	// [mailbox] tick 0 ping from client: hello
	// [mailbox] tick 2 pong to client: pong!
	// cpu stride: 1
	// memory stride: 2
	// final time: 2 cycles
}
