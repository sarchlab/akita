package endpoint

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine
//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port
//go:generate mockgen -destination "mock_simulation_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/simulation Simulation

func TestEndpoint(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Endpoint Suite")
}
