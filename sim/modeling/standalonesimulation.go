package modeling

import (
	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// standaloneSimulation provides a simulation context without monitoring or recording.
// All elements in a standalone setup must
// share one instance, just as they would share a full simulation.
type standaloneSimulation struct {
	setup  assembly.Setup
	engine timing.Engine
	ids    *timing.IDGenerator
}

// NewStandaloneSimulation creates a lightweight simulation around an engine.
// Construct it once and pass it to every builder in that simulation. This is
// useful for isolated tests and engine-only examples.
func NewStandaloneSimulation(engine timing.Engine) timing.Simulation {
	s := &standaloneSimulation{engine: engine, ids: &timing.IDGenerator{}}
	engine.SetRunGuard(s.setup.Ready)
	return s
}

func (s *standaloneSimulation) Engine() timing.Engine             { return s.engine }
func (s *standaloneSimulation) NewID() uint64                     { return s.ids.NewID() }
func (s *standaloneSimulation) RegisterComponent(e naming.Named)  { s.setup.Register(e) }
func (s *standaloneSimulation) RegisterConnection(e naming.Named) { s.setup.Register(e) }
func (s *standaloneSimulation) RegisterResource(e naming.Named)   { s.setup.Register(e) }
func (s *standaloneSimulation) RegisterPort(e naming.Named)       { s.setup.Register(e) }

func (s *standaloneSimulation) Initialize() error                { return s.setup.Initialize() }
func (s *standaloneSimulation) RequireSetup()                    { s.setup.RequireSetup() }
func (s *standaloneSimulation) RequireNameAvailable(name string) { s.setup.RequireNameAvailable(name) }
