package rob

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:           1 * timing.GHz,
		BufferSize:     128,
		NumReqPerCycle: 4,
	}
}

func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	*initialMiddlewares = middlewares{Pipeline: &middleware{comp: c}}
}
