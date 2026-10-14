// Package testsim adapts real simulation services to an engine supplied by an
// isolated component test. Production code uses sim.MakeBuilder directly.
package testsim

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Simulation routes component scheduling to the supplied test engine. Its
// embedded Simulation owns initialization, IDs, inventory, and task tracking.
// Tests execute the supplied engine directly; Simulation.Run and checkpoint
// operations belong to the embedded simulation's own engine and are not used.
type Simulation struct {
	*sim.Simulation
	engine timing.Engine
}

// New supplies simulation services to tests with a mock or instrumented engine.
func New(engine timing.Engine, tracking bool) *Simulation {
	b := sim.MakeBuilder()
	if tracking {
		b = b.WithTaskTracking()
	}
	return &Simulation{Simulation: b.Build(), engine: engine}
}

func (s *Simulation) Engine() timing.Engine { return s.engine }
