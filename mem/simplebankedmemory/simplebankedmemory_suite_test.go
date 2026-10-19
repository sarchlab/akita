package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the memory named name, with the given buffer
// sizes (each used for both the incoming and outgoing buffer).
func makePorts(simulation modeling.Simulation, name string, top, control int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: top, OutgoingCapacity: top}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: control, OutgoingCapacity: control}).
			Build(),
	}
}
