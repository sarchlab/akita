package twowaybuffered_test

import (
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

type Port = twowaybuffered.Port
type Spec = twowaybuffered.Spec

var MakeBuilder = twowaybuffered.MakeBuilder
var DefaultSpec = twowaybuffered.DefaultSpec

func newTestPort(in, out int) *Port {
	return MakeBuilder().
		WithSimulation(sim.MakeBuilder().
			Build()).
		WithSpec(Spec{IncomingCapacity: in, OutgoingCapacity: out}).
		Build()
}
