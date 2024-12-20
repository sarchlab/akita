package cellsplit

import (
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

func (e SplitEvent) Time() timing.VTimeInSec {
	return e.time
}

func (e SplitEvent) Handler() timing.Handler {
	return e.handler
}

func (e SplitEvent) IsSecondary() bool {
	return false
}

func (e SplitEvent) Serialize() (map[string]any, error) {
	return map[string]any{
		"time":    e.time,
		"handler": e.handler,
	}, nil
}

func (e *SplitEvent) Deserialize(
	data map[string]any,
) error {
	e.time = data["time"].(timing.VTimeInSec)
	e.handler = data["handler"].(timing.Handler)

	return nil
}
