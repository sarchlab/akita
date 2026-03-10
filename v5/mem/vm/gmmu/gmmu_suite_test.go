package gmmu

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
