package datamover

import (
	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// buildIdealMem builds an ideal memory controller named name, backed by
// storage, for the data mover to move data to and from.
func buildIdealMem(
	sim *testsim.Simulation,
	spec idealmemcontroller.Spec,
	storage *mem.Storage,
	name string,
) *idealmemcontroller.Comp {
	builtComponent := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).
		WithSimulation(sim).
		WithResources(idealmemcontroller.Resources{Storage: storage}).
		Build(name)

	builtComponent.BindPort("Top", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())
	builtComponent.BindPort("Control", twowaybuffered.MakeBuilder().
		WithSimulation(sim).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
		Build())
	return builtComponent
}
