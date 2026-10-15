package tickingping

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

func (initialMiddlewares *Middlewares) Initialize(c *Comp) {
	*initialMiddlewares = Middlewares{
		Send:           &sendMW{comp: c},
		ReceiveProcess: &receiveProcessMW{comp: c},
	}
}
