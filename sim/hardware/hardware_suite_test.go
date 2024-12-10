package hardware

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_hardware_test.go" -self_package github.com/sarchlab/akita/v4/sim/hardware -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/hardware Component,Port,Connection
//go:generate mockgen -destination "mock_timing_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine,Ticker
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim Buffer

func TestHardware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hardware Suite")
}
