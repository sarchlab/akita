package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Definition declares the ideal memory controller, a ticking component: its
// default configuration and its behavior. Its ports and middlewares are the
// fields of Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, state, Resources, Ports, middlewares]{
	DefaultSpec: Spec{
		Freq:          1 * timing.GHz,
		Latency:       100,
		Width:         1,
		CacheLineSize: 64,
	},
}

// Initialize creates an enabled controller with no in-flight access.
func (initialState *state) Initialize(_ *Comp) {
	*initialState = state{ControlState: memcontrolprotocol.StateEnabled}
}

// Initialize creates the controller's middlewares. It panics if the system
// builder did not supply a storage.
func (initialMiddlewares *middlewares) Initialize(c *Comp) {
	if c.Resources.Storage == nil {
		panic("idealmemcontroller: Resources.Storage is required")
	}

	*initialMiddlewares = middlewares{
		Ctrl:   &ctrlMiddleware{comp: c},
		Memory: &memMiddleware{comp: c},
	}
}
