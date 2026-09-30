package rob

import (
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the reorder buffer: its default configuration, its
// ports, and its behavior. Build a reorder buffer with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = modeling.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	Name: "ReorderBuffer",
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
