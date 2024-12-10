package simulation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_model_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v4/sim/model Port,Connection,Component

func TestSimulation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simulation Suite")
}
