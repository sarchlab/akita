package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("ValidateState(State{}) = %v, want nil", err)
	}
}

// defaultPorts creates the ports of the GMMU named name, with the historical
// default buffer sizes.
func defaultPorts(simulation modeling.Simulation, name string) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build(),
		Bottom: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
			Build(),
	}
}

// allPorts lists the GMMU's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control}
}
