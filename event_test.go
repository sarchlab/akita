package akita_test

import (
	"fmt"
	"math/rand"

	"gitlab.com/akita/akita"
)

type SplitEvent struct {
	time    akita.VTimeInSec
	handler akita.Handler
}

func (e SplitEvent) Time() akita.VTimeInSec {
	return e.time
}
func (e SplitEvent) Handler() akita.Handler {
	return e.handler
}
func (e SplitEvent) IsSecondary() bool {
	return false
}

type SplitHandler struct {
	total  int
	engine akita.Engine
}

func (h *SplitHandler) Handle(evt akita.Event) error {
	h.total++
	now := evt.Time()
	nextTime := now + akita.VTimeInSec(rand.Float64()*2+0.5)
	if nextTime < 10.0 {
		nextEvt := SplitEvent{
			time:    nextTime,
			handler: h,
		}
		h.engine.Schedule(nextEvt)
	}
	nextTime = now + akita.VTimeInSec(rand.Float64()*2+0.5)
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
	engine := akita.NewSerialEngine()
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
