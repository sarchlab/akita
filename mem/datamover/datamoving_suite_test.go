package datamover

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

func TestDataMover(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataMover Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the data mover named name, with the given
// buffer sizes (each used for both the incoming and outgoing buffer).
func makePorts(name string, top, inside, outside, control int) Ports {
	return Ports{
		Top:     messaging.NewPort(nil, top, top, name+".Top"),
		Inside:  messaging.NewPort(nil, inside, inside, name+".Inside"),
		Outside: messaging.NewPort(nil, outside, outside, name+".Outside"),
		Control: messaging.NewPort(nil, control, control, name+".Control"),
	}
}

// allPorts lists the data mover's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{
		comp.Ports.Top, comp.Ports.Inside, comp.Ports.Outside, comp.Ports.Control,
	}
}
