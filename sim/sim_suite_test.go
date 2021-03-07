package sim

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -self_package=gitlab.com/akita/akita/v2/sim -package $GOPACKAGE -write_package_comment=false gitlab.com/akita/akita/v2/sim Port,Engine,Event,Connection,Component,Handler,Ticker

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Sim")
}
