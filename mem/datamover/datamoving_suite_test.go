package datamover

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the data mover named name, with the given
// buffer sizes (each used for both the incoming and outgoing buffer).
func makePorts(simulation modeling.Simulation, name string, top, inside, outside, control int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: top, OutgoingCapacity: top}).
			Build(),
		Inside: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: inside, OutgoingCapacity: inside}).
			Build(),
		Outside: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: outside, OutgoingCapacity: outside}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: control, OutgoingCapacity: control}).
			Build(),
	}
}

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(simulation modeling.Simulation, name string, bufSize int) messaging.Port {
	p := twowaybuffered.MakeBuilder().
		WithSimulation(simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
		Build()
	p.BindOwner(testDriver{}, name)

	simulation.RegisterPort(p)
	return p
}

// allPorts lists the data mover's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{
		comp.Ports.Top, comp.Ports.Inside, comp.Ports.Outside, comp.Ports.Control,
	}
}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
