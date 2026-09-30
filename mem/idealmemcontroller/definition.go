package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the ideal memory controller, a ticking component: its
// default configuration and its behavior. Its ports and middlewares are the
// fields of Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
	DefaultSpec: Spec{
		Freq:          1 * timing.GHz,
		Latency:       100,
		Width:         1,
		CacheLineSize: 64,
	},
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState returns an enabled controller with no in-flight access.
func newState(_ *Comp) State {
	return State{ControlState: memcontrolprotocol.StateEnabled}
}

// newMiddlewares creates the controller's middlewares. It panics if the system
// builder did not supply a storage.
func newMiddlewares(c *Comp) Middlewares {
	if c.Resources().Storage == nil {
		panic("idealmemcontroller: Resources.Storage is required")
	}

	return Middlewares{
		Ctrl:   &ctrlMiddleware{comp: c},
		Memory: &memMiddleware{comp: c},
	}
}
