package cellsplit

import (
	"github.com/sarchlab/akita/v4/sim/serialization"
	"github.com/sarchlab/akita/v4/sim/timing"
)

func init() {
	serialization.RegisterType(splitEvent{})
}

type splitEvent struct {
	id      string
	time    timing.VTimeInSec
	handler timing.Handler
}

func (e splitEvent) ID() string {
	return e.id
}

func (e splitEvent) Time() timing.VTimeInSec {
	return e.time
}

func (e splitEvent) Handler() timing.Handler {
	return e.handler
}

func (e splitEvent) IsSecondary() bool {
	return false
}

func (e splitEvent) Serialize() (map[string]any, error) {
	return map[string]any{
		"id":      e.id,
		"time":    e.time,
		"handler": e.handler,
	}, nil
}

func (e splitEvent) Deserialize(
	data map[string]any,
) (serialization.Serializable, error) {
	e.id = data["id"].(string)
	e.time = data["time"].(timing.VTimeInSec)
	e.handler = data["handler"].(timing.Handler)

	return e, nil
}
