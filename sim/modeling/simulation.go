package modeling

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// Simulation supplies the runtime services used by model builders and elements.
// The concrete sim.Simulation owns initialization, execution and termination.
// TaskTracker returns nil when tracking is disabled.
type Simulation interface {
	Engine() timing.Engine
	NewID() uint64
	TaskTracker() *tasks.Tracker
	RegisterComponent(messaging.PortOwner)
	RegisterPort(messaging.Port)
	RegisterConnection(messaging.Connection)
	RegisterResource(naming.Named)
}
