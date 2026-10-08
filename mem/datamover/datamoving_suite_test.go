package datamover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestDataMover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataMover Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the data mover named name, with the given
// buffer sizes (each used for both the incoming and outgoing buffer).
func makePorts(name string, top, inside, outside, control int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(top, top),
		Inside:  twowaybuffered.NewPort(inside, inside),
		Outside: twowaybuffered.NewPort(outside, outside),
		Control: twowaybuffered.NewPort(control, control),
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
	p := twowaybuffered.NewPort(bufSize, bufSize)
	p.BindOwner(testDriver{}, name)

	return p
}

// allPorts lists the data mover's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{
		comp.Ports.Top, comp.Ports.Inside, comp.Ports.Outside, comp.Ports.Control,
	}
}
