package gmmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

func TestGMMU(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GMMU Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("ValidateState(State{}) = %v, want nil", err)
	}
}

// defaultPorts creates the ports of the GMMU named name, with the historical
// default buffer sizes.
func defaultPorts(name string) Ports {
	return Ports{
		Top:     messaging.NewPort(nil, 16, 16, name+".Top"),
		Bottom:  messaging.NewPort(nil, 16, 16, name+".Bottom"),
		Control: messaging.NewPort(nil, 4, 4, name+".Control"),
	}
}

// allPorts lists the GMMU's ports.
func allPorts(comp *Comp) []messaging.Port {
	return []messaging.Port{comp.Ports.Top, comp.Ports.Bottom, comp.Ports.Control}
}
