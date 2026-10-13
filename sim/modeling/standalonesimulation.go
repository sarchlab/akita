package modeling

import (
	"github.com/sarchlab/akita/v5/internal/assembly"
	"github.com/sarchlab/akita/v5/sim/internal/taskcore"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// StandaloneSimulation provides a simulation context without monitoring or recording.
// All elements in a standalone setup must
// share one instance, just as they would share a full simulation.
type StandaloneSimulation struct {
	setup           assembly.Setup
	engine          timing.Engine
	ids             *timing.IDGenerator
	taskTracker     *tasks.Tracker
	componentsBuilt bool
}

// NewStandaloneSimulation creates a lightweight simulation around an engine.
// Construct it once and pass it to every builder in that simulation. This is
// useful for isolated tests and engine-only examples.
func NewStandaloneSimulation(engine timing.Engine) *StandaloneSimulation {
	s := &StandaloneSimulation{engine: engine, ids: &timing.IDGenerator{}}
	engine.SetRunGuard(func() error {
		if err := s.setup.Ready(); err != nil {
			return err
		}
		taskcore.FreezeSubscriptions(s.taskTracker)
		return nil
	})
	return s
}

func (s *StandaloneSimulation) Engine() timing.Engine { return s.engine }
func (s *StandaloneSimulation) NewID() uint64         { return s.ids.NewID() }
func (s *StandaloneSimulation) RegisterComponent(e naming.Named) {
	s.setup.Register(e)
	s.componentsBuilt = true
}
func (s *StandaloneSimulation) RegisterConnection(e naming.Named) { s.setup.Register(e) }
func (s *StandaloneSimulation) RegisterResource(e naming.Named)   { s.setup.Register(e) }
func (s *StandaloneSimulation) RegisterPort(e naming.Named) {
	s.setup.Register(e)
	if p, ok := e.(messaging.Port); ok {
		taskcore.RegisterPort(s.taskTracker, p)
	}
}

func (s *StandaloneSimulation) Initialize() error                { return s.setup.Initialize() }
func (s *StandaloneSimulation) RequireSetup()                    { s.setup.RequireSetup() }
func (s *StandaloneSimulation) RequireNameAvailable(name string) { s.setup.RequireNameAvailable(name) }

// WithTaskTracking enables typed task observations for an engine-only setup.
// Call before building components so each owner receives the same tracker.
func (s *StandaloneSimulation) WithTaskTracking() *StandaloneSimulation {
	s.setup.RequireSetup()
	if s.taskTracker == nil {
		if s.componentsBuilt {
			panic("modeling: enable task tracking before building components")
		}
		s.taskTracker = taskcore.NewTracker()
	}
	return s
}

// Tasks returns the optional task service.
func (s *StandaloneSimulation) Tasks() *tasks.Tracker { return s.taskTracker }
