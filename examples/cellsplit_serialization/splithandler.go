package cellsplit

import (
	"math/rand"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/simulation"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&state{}))
}

type state struct {
	name    string
	endTime timing.VTimeInSec
	total   int
}

func (s *state) Name() string {
	return s.name
}

func (s *state) Serialize() (map[string]any, error) {
	return map[string]any{
		"endTime": s.endTime,
		"total":   s.total,
	}, nil
}

func (s *state) Deserialize(m map[string]any) error {
	s.endTime = m["endTime"].(timing.VTimeInSec)
	s.total = m["total"].(int)

	return nil
}

type SplitHandler struct {
	*state

	engine timing.Engine
	rand   *rand.Rand
}

func (h *SplitHandler) Name() string {
	return h.name
}

func (h *SplitHandler) State() simulation.State {
	return h.state
}

func (h *SplitHandler) SetState(s simulation.State) {
	h.state = s.(*state)
}

func (h *SplitHandler) Handle(evt timing.Event) error {
	h.total++
	now := evt.Time()
	nextTime := now + timing.VTimeInSec(h.rand.Float64()*2+0.5)

	if nextTime < h.endTime {
		nextEvt := splitEvent{
			id:      id.Generate(),
			time:    nextTime,
			handler: h,
		}

		h.engine.Schedule(&nextEvt)
	}

	return nil
}
