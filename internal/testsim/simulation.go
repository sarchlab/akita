// Package testsim constructs real simulation services around an engine supplied
// by a component test. Production code uses sim.MakeBuilder directly.
package testsim

import (
	"github.com/sarchlab/akita/v5/internal/simconfig"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Simulation uses one engine for scheduling, Run, and checkpoints.
type Simulation = sim.Simulation

// New supplies simulation services to tests with a mock or instrumented engine.
func New(engine timing.Engine, tracking bool) *Simulation {
	if engine == nil {
		panic("testsim: engine is required")
	}
	b := sim.MakeBuilder()
	if tracking {
		b = b.WithTaskTracking()
	}
	simconfig.SetEngine(&b, engine)
	return b.Build()
}
