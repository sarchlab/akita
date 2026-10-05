package switches

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/simulation/modeling"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/simulation/messaging Port
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/simulation/timing Engine
//go:generate mockgen -destination "mock_routing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v5/noc/networking/routing Table

func TestSwitches(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switches Suite")
}

func TestValidateState(t *testing.T) {
	if err := modeling.ValidateState(state{}); err != nil {
		t.Fatalf("State failed validation: %v", err)
	}
}
