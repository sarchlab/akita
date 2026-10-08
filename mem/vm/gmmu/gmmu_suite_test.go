package gmmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestGMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GMMU Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("ValidateState(State{}) = %v, want nil", err)
	}
}

// defaultPorts creates the ports of the GMMU named name, with the historical
// default buffer sizes.
func defaultPorts(name string) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(name+".Top", 16, 16),
		Bottom:  twowaybuffered.NewPort(name+".Bottom", 16, 16),
		Control: twowaybuffered.NewPort(name+".Control", 4, 4),
	}
}

// allPorts lists the GMMU's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control}
}
