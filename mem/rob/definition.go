package rob

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the reorder buffer, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:           1 * timing.GHz,
		BufferSize:     128,
		NumReqPerCycle: 4,
	},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(c *Comp) Middlewares {
	return Middlewares{Pipeline: &middleware{comp: c}}
}
