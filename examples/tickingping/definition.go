package tickingping

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{Freq: 1 * timing.GHz}
}

func (m *Middlewares) Initialize(c *Comp) {
	*m = Middlewares{
		Send:           &sendMW{comp: c},
		ReceiveProcess: &receiveProcessMW{comp: c},
	}
}
