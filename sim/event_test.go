package sim_test

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v3/sim"
)

type SplitEvent struct {
	time    sim.VTimeInSec
	handler sim.Handler
}

func (e SplitEvent) Time() sim.VTimeInSec {
	return e.time
}
func (e SplitEvent) Handler() sim.Handler {
	return e.handler
}
func (e SplitEvent) IsSecondary() bool {
	return false
}

type SplitHandler struct {
	total  int
	engine sim.Engine
}

func (h *SplitHandler) Handle(evt sim.Event) error {
	h.total++
	now := evt.Time()
	nextTime := now + sim.VTimeInSec(rand.Float64()*2+0.5)
	if nextTime < 10.0 {
		nextEvt := SplitEvent{
			time:    nextTime,
			handler: h,
		}
		h.engine.Schedule(nextEvt)
	}
	nextTime = now + sim.VTimeInSec(rand.Float64()*2+0.5)
	if nextTime < 10.0 {
		nextEvt := SplitEvent{
			time:    nextTime,
			handler: h,
		}
		h.engine.Schedule(nextEvt)
	}
	return nil
}

func ExampleEvent() {
	rand.Seed(1)
	engine := sim.NewSerialEngine()
	splitHandler := SplitHandler{
		total:  0,
		engine: engine,
	}
	engine.Schedule(SplitEvent{
		time:    0,
		handler: &splitHandler,
	})
	engine.Run()
	fmt.Printf("Total number at time 10: %d\n", splitHandler.total)
	// Output: Total number at time 10: 185
}
