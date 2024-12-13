package switches

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine
//go:generate mockgen -destination "mock_modeling_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/modeling Port
//go:generate mockgen -destination "mock_queueing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/queueing Buffer,Pipeline
//go:generate mockgen -destination "mock_routing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/noc/networking/routing Table
//go:generate mockgen -destination "mock_arbitration_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/noc/networking/arbitration Arbiter

func TestSwitches(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switches Suite")
}
