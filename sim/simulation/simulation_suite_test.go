package simulation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -self_package=github.com/sarchlab/akita/v4/sim/simulation -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/hardware Port,Connection,Component

func TestSimulation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simulation Suite")
}
