package modeling

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_modeling_test.go" -self_package github.com/sarchlab/akita/v4/sim/modeling -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Component,Port,Connection
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine,Ticker
//go:generate mockgen -destination "mock_queueing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/queueing Buffer
//go:generate mockgen -destination "mock_simulation_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/simulation Simulation

func TestModeling(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Modeling Suite")
}
