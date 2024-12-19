package cellsplit

import (
	"math/rand"
	"reflect"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func init() {
	serialization.RegisterType(reflect.TypeOf(&SplitHandler{}))
}

type SplitHandler struct {
	name   string
	engine timing.Engine
	rand   *rand.Rand

	endTime timing.VTimeInSec
	total   int
}

func (h *SplitHandler) ID() string {
	return h.name
}

func (h *SplitHandler) Serialize() (map[string]any, error) {
	return map[string]any{
		"endTime": h.endTime,
		"total":   h.total,
	}, nil
}

func (h *SplitHandler) Deserialize(
	m map[string]any,
) error {
	h.endTime = m["endTime"].(timing.VTimeInSec)
	h.total = m["total"].(int)

	return nil
}

func (h *SplitHandler) Name() string {
	return h.name
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
