package modeling

import (
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// Registrar is the build-time context a builder needs in order to register the
// entity it builds. A *simulation.Simulation satisfies it.
//
// A builder accepts a Registrar through a WithSimulation method so that building
// an entity both sources its engine (GetEngine, for components and connections)
// and registers the entity in a single step. The lower-level WithEngine path
// remains available for isolated unit tests that do not need a full simulation.
// Each builder calls the registration method matching the kind it builds.
type Registrar interface {
	GetEngine() timing.Engine
	RegisterComponent(c naming.Named)
	RegisterConnection(c naming.Named)
	RegisterResource(c naming.Named)
}
