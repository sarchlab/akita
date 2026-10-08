package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimpleBankedMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleBankedMemory Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the memory named name, with the given buffer
// sizes (each used for both the incoming and outgoing buffer).
func makePorts(name string, top, control int) Ports {
	return Ports{
		Top:     twowaybuffered.NewPort(name+".Top", top, top),
		Control: twowaybuffered.NewPort(name+".Control", control, control),
	}
}
