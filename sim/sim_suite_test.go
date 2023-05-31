package sim

import (
	"log"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -self_package=github.com/sarchlab/akita/v3/sim -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port,Engine,Event,Connection,Component,Handler,Ticker,Buffer

func TestCore(t *testing.T) {
	log.SetOutput(ginkgo.GinkgoWriter)
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Sim Suite")
}
