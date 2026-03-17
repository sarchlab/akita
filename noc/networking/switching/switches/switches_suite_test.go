package switches

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate go run go.uber.org/mock/mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Port,Engine,Buffer
//go:generate go run go.uber.org/mock/mockgen -destination "mock_pipelining_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/pipelining Pipeline
//go:generate go run go.uber.org/mock/mockgen -destination "mock_routing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/noc/networking/routing Table
//go:generate go run go.uber.org/mock/mockgen -destination "mock_arbitration_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/noc/networking/arbitration Arbiter

func TestSwitches(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switches Suite")
}
