package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/sim Port,Engine
func TestTlb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tlb Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(State{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
