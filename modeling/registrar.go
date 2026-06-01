package modeling

import (
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// Registrar is the build-time context a component builder needs in order to
// register the component it builds. A *simulation.Simulation satisfies it.
//
// A builder accepts a Registrar through a WithSimulation method so that building
// a component both sources its engine (GetEngine) and registers the component
// (RegisterComponent) in a single step. The lower-level WithEngine path remains
// available for isolated unit tests that do not need a full simulation.
type Registrar interface {
	GetEngine() timing.Engine
	RegisterComponent(c naming.Named)
}
