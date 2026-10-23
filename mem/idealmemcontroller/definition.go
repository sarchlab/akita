package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:           1 * timing.GHz,
		Latency:        100,
		NumReqPerCycle: 1,
	}
}

// Initialize creates an enabled controller with no in-flight access.
func (s *state) Initialize(_ *Comp) {
	*s = state{ControlState: controlStateEnabled}
}

// Initialize creates the controller's middlewares. It panics if the system
// builder did not supply a storage.
func (m *middlewares) Initialize(c *Comp) {
	if c.Spec.Latency < 0 {
		panic("idealmemcontroller: Latency must be nonnegative")
	}
	if c.Resources.Storage == nil {
		panic("idealmemcontroller: Resources.Storage is required")
	}

	*m = middlewares{
		Ctrl:   &ctrlMiddleware{comp: c},
		Memory: &memMiddleware{comp: c},
	}
}
