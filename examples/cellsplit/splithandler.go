package cellsplit

import (
	"math/rand"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/timing"
)

type SplitHandler struct {
	engine timing.Engine
	rand   *rand.Rand
	total  int
}

func (h *SplitHandler) Handle(evt timing.Event) error {
	h.total++
	now := evt.Time()
	nextTime := now + timing.VTimeInSec(h.rand.Float64()*2+0.5)

	if nextTime < 100.0 {
		nextEvt := splitEvent{
			id:      id.Generate(),
			time:    nextTime,
			handler: h,
		}

		h.engine.Schedule(nextEvt)
	}

	return nil
}
