package queueing

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_simulation_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/simulation Simulation

func TestQueueing(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Queueing Suite")
}
