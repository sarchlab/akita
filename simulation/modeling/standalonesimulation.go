package modeling

import (
	"github.com/sarchlab/akita/v5/simulation/naming"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// standaloneSimulation provides a simulation context without recording,
// monitoring, or an entity inventory. All elements in a standalone setup must
// share one instance, just as they would share a full simulation.
type standaloneSimulation struct {
	engine timing.Engine
	ids    *timing.IDGenerator
}

// NewStandaloneSimulation creates a lightweight simulation around an engine.
// Construct it once and pass it to every builder in that simulation. This is
// useful for isolated tests and engine-only examples.
func NewStandaloneSimulation(engine timing.Engine) timing.Simulation {
	return &standaloneSimulation{engine: engine, ids: &timing.IDGenerator{}}
}

func (s *standaloneSimulation) Engine() timing.Engine             { return s.engine }
func (s *standaloneSimulation) NewID() uint64                     { return s.ids.NewID() }
func (s *standaloneSimulation) RegisterComponent(_ naming.Named)  {}
func (s *standaloneSimulation) RegisterConnection(_ naming.Named) {}
func (s *standaloneSimulation) RegisterResource(_ naming.Named)   {}
func (s *standaloneSimulation) RegisterPort(_ naming.Named)       {}
