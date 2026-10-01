package simplebankedmemory

import (
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimpleBankedMemory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleBankedMemory Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}

// makePorts creates the ports of the memory named name, with the given buffer
// sizes (each used for both the incoming and outgoing buffer).
func makePorts(name string, top, control int) Ports {
	return Ports{
		Top:     messaging.NewPort(name+".Top", top, top),
		Control: messaging.NewPort(name+".Control", control, control),
	}
}
