package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// defaultPorts creates the ports of the DRAM controller named name, each with
// the given buffer size.
func defaultPorts(simulation modeling.Simulation, name string, bufSize int) Ports {
	return Ports{
		Top: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
			Build(),
		Control: twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
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

// storageFor returns a storage that covers the address space of the geometry
// in spec.
func storageFor(spec Spec) *mem.Storage {
	devicePerRank := spec.BusWidth / spec.DeviceWidth
	bankSize := spec.NumCol * spec.NumRow * spec.DeviceWidth / 8
	rankSize := bankSize * spec.NumBank * devicePerRank
	totalSize := rankSize * spec.NumRank * spec.NumChannel

	return mem.NewStorage(uint64(totalSize))
}

// buildDRAM builds a DRAM controller named name from spec, backed by a
// storage that covers its geometry, with ports of the given buffer size.
func buildDRAM(sim *testsim.Simulation, spec Spec, name string, bufSize int) *Comp {
	builtComponent := modeling.MakeBuilder[*Comp](spec).
		WithSimulation(sim).
		WithResources(Resources{Storage: storageFor(spec)}).
		Build(name)
	builtComponentPorts := defaultPorts(sim, name, bufSize)
	builtComponent.BindPort("Top", builtComponentPorts.Top)
	builtComponent.BindPort("Control", builtComponentPorts.Control)
	return builtComponent
}

func (o testDriver) Name() string                           { return "TestDriver" }
func (o testDriver) BindPort(slot string, p messaging.Port) { p.BindOwner(o, o.Name()+"."+slot) }
