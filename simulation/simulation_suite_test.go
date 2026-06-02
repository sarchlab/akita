package simulation

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSimulation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Simulation Suite")
}
