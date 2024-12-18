package timing_test

import (
	"fmt"
	"math/rand"

	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type SplitEvent struct {
	id      string
	time    timing.VTimeInSec
	handler timing.Handler
}

func (e SplitEvent) ID() string {
	return e.id
}

func (e SplitEvent) Serialize() (map[string]any, error) {
	return map[string]any{
		"id":   e.id,
		"time": e.time,
	}, nil
}

func (e SplitEvent) Deserialize(
	data map[string]any,
) (serialization.Serializable, error) {
	e.id = data["id"].(string)
	e.time = data["time"].(timing.VTimeInSec)

	return e, nil
}

func (e SplitEvent) Time() timing.VTimeInSec {
	return e.time
}
func (e SplitEvent) Handler() timing.Handler {
	return e.handler
}
func (e SplitEvent) IsSecondary() bool {
	return false
}

type SplitHandler struct {
	total  int
	engine timing.Engine
}

func (h *SplitHandler) Handle(evt timing.Event) error {
	h.total++
	now := evt.Time()
	nextTime := now + timing.VTimeInSec(rand.Float64()*2+0.5)

	if nextTime < 10.0 {
		nextEvt := SplitEvent{
			time:    nextTime,
			handler: h,
		}
		h.engine.Schedule(nextEvt)
	}

	nextTime = now + timing.VTimeInSec(rand.Float64()*2+0.5)
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

	engine := timing.NewSerialEngine()

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
