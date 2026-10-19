package twowaybuffered

import "github.com/sarchlab/akita/v5/sim"

func newTestPort(in, out int) *Port {
	return MakeBuilder().
		WithSimulation(sim.MakeBuilder().
			Build()).
		WithSpec(Spec{IncomingCapacity: in, OutgoingCapacity: out}).
		Build()
}
