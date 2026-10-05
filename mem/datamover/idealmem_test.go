package datamover

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/timing"
)

// buildIdealMem builds an ideal memory controller named name, backed by
// storage, for the data mover to move data to and from.
func buildIdealMem(
	sim timing.Simulation,
	spec idealmemcontroller.Spec,
	storage *mem.Storage,
	name string,
) *idealmemcontroller.Comp {
	return idealmemcontroller.Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(idealmemcontroller.Resources{Storage: storage}).
		WithPorts(idealmemcontroller.Ports{
			Top:     messaging.NewPort(name+".Top", 16, 16),
			Control: messaging.NewPort(name+".Control", 16, 16),
		}).
		Build(name)
}
