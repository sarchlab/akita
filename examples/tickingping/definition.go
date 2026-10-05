package tickingping

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the tickingping component, a ticking component: its
// default configuration and its behavior. Its ports and middlewares are the
// fields of Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name).
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Freq: 1 * timing.GHz},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{
		Send:           &sendMW{comp: c},
		ReceiveProcess: &receiveProcessMW{comp: c},
	}
}
