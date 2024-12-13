package timing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_timing_test.go" -self_package=github.com/sarchlab/akita/v4/sim/timing -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/timing Engine,Event,Handler,Ticker

func TestTiming(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Timing Suite")
}
