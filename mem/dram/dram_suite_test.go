package dram

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dram Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// defaultPorts creates the ports of the DRAM controller named name, each with
// the given buffer size.
func defaultPorts(name string, bufSize int) Ports {
	return Ports{
		Top:     messaging.NewPort(name+".Top", bufSize, bufSize),
		Control: messaging.NewPort(name+".Control", bufSize, bufSize),
	}
}

// testDriver owns the ports a test drives by hand. The test polls those ports,
// so it ignores the notifications.
type testDriver struct{}

func (testDriver) NotifyRecv(messaging.Port)     {}
func (testDriver) NotifyPortFree(messaging.Port) {}

// newDriverPort creates a port with bufSize slots in each direction for the
// test to drive by hand.
func newDriverPort(name string, bufSize int) messaging.Port {
	p := messaging.NewPort(name, bufSize, bufSize)
	p.SetOwner(testDriver{})

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
func buildDRAM(sim timing.Simulation, spec Spec, name string, bufSize int) *Comp {
	return Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(Resources{Storage: storageFor(spec)}).
		WithPorts(defaultPorts(name, bufSize)).
		Build(name)
}
